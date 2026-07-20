package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
)

const minPasswordLen = 8

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Sub   string `json:"sub"` // user id
	Email string `json:"email"`
	Role  string `json:"role"`
	Exp   int64  `json:"exp"`
	Iat   int64  `json:"iat"`
}

// UserID parses the subject claim into a user id.
func (c *jwtClaims) UserID() int64 {
	id, _ := strconv.ParseInt(c.Sub, 10, 64)
	return id
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResp struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

type loginResponse struct {
	Token     string   `json:"token"`
	ExpiresAt int64    `json:"expires_at"`
	User      userResp `json:"user"`
}

func toUserResp(u *store.User) userResp {
	return userResp{ID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role, CreatedAt: u.CreatedAt.Format(time.RFC3339)}
}

// handleAuthStatus reports whether initial setup (first admin) is required.
func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.CountUsers(r.Context())
	if err != nil {
		s.log.Error("count users", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"setup_required": n == 0})
}

// handleSetup creates the first admin account. Only allowed when no users exist.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	n, err := s.store.CountUsers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if n > 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "setup already completed"})
		return
	}

	var req credentials
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if msg := validateCredentials(req); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}

	user, err := s.createUser(r, req.Email, req.Password, store.GlobalRoleSuperadmin)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create admin"})
		return
	}
	// The first account is the platform superadmin, who manages every project and
	// is deliberately not attached to any single one — so no personal project is
	// provisioned. They create and pick projects from the global superadmin surfaces.
	s.issueToken(w, user)
}

// handleLogin authenticates a user by email + password.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req credentials
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	user, err := s.store.GetUserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	// Always run a bcrypt comparison to reduce user-enumeration timing signal.
	hash := dummyHash
	if user != nil {
		hash = user.PasswordHash
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
		return
	}

	s.issueToken(w, user)
}

// handleMe returns the current authenticated user.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	user, err := s.store.GetUserByID(r.Context(), claims.UserID())
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, toUserResp(user))
}

type profileUpdate struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// handleUpdateProfile updates the current user's own name and email.
func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req profileUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !strings.Contains(email, "@") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a valid email is required"})
		return
	}

	// Email must stay unique.
	if existing, err := s.store.GetUserByEmail(r.Context(), email); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	} else if existing != nil && existing.ID != claims.UserID() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "that email is already in use"})
		return
	}

	if err := s.store.UpdateUserProfile(r.Context(), claims.UserID(), strings.TrimSpace(req.Name), email); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update profile"})
		return
	}
	user, _ := s.store.GetUserByID(r.Context(), claims.UserID())
	writeJSON(w, http.StatusOK, toUserResp(user))
}

// handleMyStats returns the current user's own activity counts.
func (s *Server) handleMyStats(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	a, err := s.store.GetUserActivity(r.Context(), claims.UserID())
	if err != nil {
		s.log.Error("user activity", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load activity"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{
		"runs":         a.Runs,
		"total_tokens": a.TotalTokens,
		"reminders":    a.Reminders,
		"notes":        a.Notes,
	})
}

type passwordChange struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// handleChangePassword updates the current user's own password.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req passwordChange
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if len(req.NewPassword) < minPasswordLen {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("password must be at least %d characters", minPasswordLen)})
		return
	}

	user, err := s.store.GetUserByID(r.Context(), claims.UserID())
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)) != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "current password is incorrect"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if err := s.store.UpdateUserPassword(r.Context(), user.ID, string(hash)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update password"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type forgotPasswordReq struct {
	Email string `json:"email"`
}

// handleForgotPassword resets the password for the account matching the given
// email: it generates a new random password, emails the plaintext to the user,
// and only then persists the new hash. The user signs in with it and changes it
// from their profile page.
//
// For an unknown email it still responds 200 with a generic body, so the
// endpoint does not reveal whether an account exists for a given address.
func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req forgotPasswordReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !strings.Contains(email, "@") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a valid email is required"})
		return
	}

	if s.mailer == nil || !s.mailer.Enabled() {
		s.log.Error("forgot-password requested but SMTP is not configured")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "password reset by email is not available"})
		return
	}

	user, err := s.store.GetUserByEmail(r.Context(), email)
	if err != nil {
		s.log.Error("forgot-password lookup", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if user == nil {
		// Unknown email — respond OK without sending, to avoid user enumeration.
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	newPassword, err := generateRandomPassword(16)
	if err != nil {
		s.log.Error("generate password", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Send the email *before* persisting the new hash: if delivery fails, the
	// user keeps their old password rather than being locked out of an account
	// whose password was changed to one they never received.
	if err := s.sendResetEmail(user, newPassword); err != nil {
		s.log.Error("send reset email", "error", err, "user", user.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not send the reset email, please try again later"})
		return
	}

	hash, err := hashPassword(newPassword)
	if err != nil {
		s.log.Error("hash password", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if err := s.store.UpdateUserPassword(r.Context(), user.ID, hash); err != nil {
		s.log.Error("update password", "error", err, "user", user.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reset password"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) sendResetEmail(user *store.User, newPassword string) error {
	name := strings.TrimSpace(user.Name)
	if name == "" {
		name = "there"
	}
	subject := "Your Personal Assistant password has been reset"
	body := fmt.Sprintf(`Hi %s,

We received a request to reset your Personal Assistant password.

Your new temporary password is:

    %s

Sign in with it, then change it right away from your profile page.

If you did not request this, you can ignore this email — but consider changing
your password if you are concerned.
`, name, newPassword)
	return s.mailer.Send(user.Email, subject, body)
}

// passwordAlphabet omits visually ambiguous characters (0/O, 1/l/I) so a
// password read from an email is easy to transcribe.
const passwordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

// generateRandomPassword returns a cryptographically random password of n
// characters drawn from passwordAlphabet.
func generateRandomPassword(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i, b := range buf {
		buf[i] = passwordAlphabet[int(b)%len(passwordAlphabet)]
	}
	return string(buf), nil
}

func hashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

func (s *Server) createUser(r *http.Request, email, password, role string) (*store.User, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}
	return s.store.CreateUser(r.Context(), strings.ToLower(strings.TrimSpace(email)), hash, role)
}

func (s *Server) issueToken(w http.ResponseWriter, user *store.User) {
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	token, err := s.generateToken(user, expiresAt)
	if err != nil {
		s.log.Error("failed to generate token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Token: token, ExpiresAt: expiresAt.Unix(), User: toUserResp(user)})
}

func validateCredentials(c credentials) string {
	if !strings.Contains(c.Email, "@") {
		return "a valid email is required"
	}
	if len(c.Password) < minPasswordLen {
		return fmt.Sprintf("password must be at least %d characters", minPasswordLen)
	}
	return ""
}

// dummyHash is a valid bcrypt hash used for constant-work comparison on unknown emails.
var dummyHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

func (s *Server) generateToken(user *store.User, expiresAt time.Time) (string, error) {
	header := jwtHeader{Alg: "HS256", Typ: "JWT"}
	claims := jwtClaims{
		Sub:   strconv.FormatInt(user.ID, 10),
		Email: user.Email,
		Role:  user.Role,
		Exp:   expiresAt.Unix(),
		Iat:   time.Now().Unix(),
	}

	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	payload := headerB64 + "." + claimsB64
	signature := s.sign([]byte(payload))
	sigB64 := base64.RawURLEncoding.EncodeToString(signature)

	return payload + "." + sigB64, nil
}

func (s *Server) validateToken(tokenStr string) (*jwtClaims, error) {
	parts := strings.SplitN(tokenStr, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	payload := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding")
	}

	expectedSig := s.sign([]byte(payload))
	if !hmac.Equal(sig, expectedSig) {
		return nil, fmt.Errorf("invalid signature")
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid claims encoding")
	}

	var claims jwtClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("invalid claims: %w", err)
	}

	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

func (s *Server) sign(data []byte) []byte {
	mac := hmac.New(sha256.New, s.signingKey)
	mac.Write(data)
	return mac.Sum(nil)
}
