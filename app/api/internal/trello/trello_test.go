package trello

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestUpdateCardSendsPut(t *testing.T) {
	var gotMethod, gotPath string
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(`{"id":"c1","name":"New title","desc":"Body","idList":"L2","shortUrl":"https://trello.com/c/x"}`))
	}))
	defer srv.Close()
	orig := base
	base = srv.URL
	defer func() { base = orig }()

	c := New()
	name := "New title"
	desc := "Body"
	list := "L2"
	labels := []string{"lbl1"}
	card, err := c.UpdateCard(context.Background(), "k", "t", "c1", UpdateCardInput{
		Name:     &name,
		Desc:     &desc,
		IDList:   &list,
		LabelIDs: &labels,
	})
	if err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotPath != "/cards/c1" {
		t.Errorf("path = %s, want /cards/c1", gotPath)
	}
	if gotQuery.Get("name") != "New title" {
		t.Errorf("name = %q", gotQuery.Get("name"))
	}
	if gotQuery.Get("desc") != "Body" {
		t.Errorf("desc = %q", gotQuery.Get("desc"))
	}
	if gotQuery.Get("idList") != "L2" {
		t.Errorf("idList = %q", gotQuery.Get("idList"))
	}
	if gotQuery.Get("idLabels") != "lbl1" {
		t.Errorf("idLabels = %q", gotQuery.Get("idLabels"))
	}
	if gotQuery.Get("key") != "k" || gotQuery.Get("token") != "t" {
		t.Errorf("auth not set: key=%q token=%q", gotQuery.Get("key"), gotQuery.Get("token"))
	}
	if card.Name != "New title" {
		t.Errorf("returned card name = %q", card.Name)
	}
}

// UpdateCard sends only the fields that are set; a nil field must be absent, and
// an empty LabelIDs slice must send an explicit empty idLabels to clear labels.
func TestUpdateCardPartialAndClearLabels(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(`{"id":"c1"}`))
	}))
	defer srv.Close()
	orig := base
	base = srv.URL
	defer func() { base = orig }()

	c := New()
	empty := []string{}
	if _, err := c.UpdateCard(context.Background(), "k", "t", "c1", UpdateCardInput{LabelIDs: &empty}); err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	if _, ok := gotQuery["name"]; ok {
		t.Error("name should be absent when Name is nil")
	}
	if _, ok := gotQuery["idList"]; ok {
		t.Error("idList should be absent when IDList is nil")
	}
	if v, ok := gotQuery["idLabels"]; !ok || v[0] != "" {
		t.Errorf("idLabels should be present and empty to clear labels, got %v (present=%v)", v, ok)
	}
}

// An update that changes nothing must not issue a PUT; it should read the card
// back (GET) so callers still get the current card.
func TestUpdateCardEmptyIsGet(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_, _ = w.Write([]byte(`{"id":"c1","name":"unchanged"}`))
	}))
	defer srv.Close()
	orig := base
	base = srv.URL
	defer func() { base = orig }()

	c := New()
	card, err := c.UpdateCard(context.Background(), "k", "t", "c1", UpdateCardInput{})
	if err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("empty update method = %s, want GET", gotMethod)
	}
	if card.Name != "unchanged" {
		t.Errorf("card name = %q", card.Name)
	}
}

// GetCard must ask Trello for the closed + idList fields (so read-after-write
// can tell an archived or misplaced card from a persisted one) and decode them.
func TestGetCardFetchesClosedAndList(t *testing.T) {
	var gotFields string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFields = r.URL.Query().Get("fields")
		_, _ = w.Write([]byte(`{"id":"c1","name":"Task","idList":"L1","idBoard":"B1","closed":true}`))
	}))
	defer srv.Close()
	orig := base
	base = srv.URL
	defer func() { base = orig }()

	c := New()
	card, err := c.GetCard(context.Background(), "k", "t", "c1")
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	for _, f := range []string{"idList", "idBoard", "closed"} {
		if !strings.Contains(gotFields, f) {
			t.Errorf("fields %q missing %q", gotFields, f)
		}
	}
	if card.IDList != "L1" {
		t.Errorf("idList = %q, want L1", card.IDList)
	}
	if card.IDBoard != "B1" {
		t.Errorf("idBoard = %q, want B1", card.IDBoard)
	}
	if !card.Closed {
		t.Error("closed should decode as true")
	}
}

// ArchiveCard archives a card by sending PUT /cards/{id} with closed=true (the
// same mechanism Trello's UI "Archive" uses), and decodes the returned card.
func TestArchiveCardSendsClosedPut(t *testing.T) {
	var gotMethod, gotPath string
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(`{"id":"c1","name":"Task","closed":true,"shortUrl":"https://trello.com/c/x"}`))
	}))
	defer srv.Close()
	orig := base
	base = srv.URL
	defer func() { base = orig }()

	card, err := New().ArchiveCard(context.Background(), "k", "t", "c1")
	if err != nil {
		t.Fatalf("ArchiveCard: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotPath != "/cards/c1" {
		t.Errorf("path = %s, want /cards/c1", gotPath)
	}
	if gotQuery.Get("closed") != "true" {
		t.Errorf("closed = %q, want true", gotQuery.Get("closed"))
	}
	if gotQuery.Get("key") != "k" || gotQuery.Get("token") != "t" {
		t.Errorf("auth not set: key=%q token=%q", gotQuery.Get("key"), gotQuery.Get("token"))
	}
	if !card.Closed {
		t.Error("returned card should be closed")
	}
}

func TestCardChecklistsAndDelete(t *testing.T) {
	var listMethod, listPath, delMethod, delPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listMethod, listPath = r.Method, r.URL.Path
			_, _ = w.Write([]byte(`[{"id":"cl1","name":"Acceptance Criteria"}]`))
		case http.MethodDelete:
			delMethod, delPath = r.Method, r.URL.Path
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	orig := base
	base = srv.URL
	defer func() { base = orig }()

	c := New()
	cls, err := c.CardChecklists(context.Background(), "k", "t", "c1")
	if err != nil {
		t.Fatalf("CardChecklists: %v", err)
	}
	if listMethod != http.MethodGet || listPath != "/cards/c1/checklists" {
		t.Errorf("checklists request = %s %s", listMethod, listPath)
	}
	if len(cls) != 1 || cls[0].ID != "cl1" || cls[0].Name != "Acceptance Criteria" {
		t.Fatalf("checklists = %+v", cls)
	}
	if err := c.DeleteChecklist(context.Background(), "k", "t", "cl1"); err != nil {
		t.Fatalf("DeleteChecklist: %v", err)
	}
	if delMethod != http.MethodDelete || delPath != "/checklists/cl1" {
		t.Errorf("delete request = %s %s", delMethod, delPath)
	}
}

func TestOrganizations(t *testing.T) {
	var gotPath string
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(`[{"id":"o1","name":"acme","displayName":"Acme Inc","url":"https://trello.com/acme"}]`))
	}))
	defer srv.Close()
	orig := base
	base = srv.URL
	defer func() { base = orig }()

	orgs, err := New().Organizations(context.Background(), "k", "t")
	if err != nil {
		t.Fatalf("Organizations: %v", err)
	}
	if gotPath != "/members/me/organizations" {
		t.Errorf("path = %s", gotPath)
	}
	if gotQuery.Get("key") != "k" || gotQuery.Get("token") != "t" {
		t.Errorf("auth not set: key=%q token=%q", gotQuery.Get("key"), gotQuery.Get("token"))
	}
	if len(orgs) != 1 || orgs[0].ID != "o1" || orgs[0].DisplayName != "Acme Inc" {
		t.Errorf("orgs = %+v", orgs)
	}
}

func TestOrganizationBoards(t *testing.T) {
	var gotPath string
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(`[{"id":"b1","name":"Roadmap","url":"https://trello.com/b/b1","closed":false}]`))
	}))
	defer srv.Close()
	orig := base
	base = srv.URL
	defer func() { base = orig }()

	boards, err := New().OrganizationBoards(context.Background(), "k", "t", "o1")
	if err != nil {
		t.Fatalf("OrganizationBoards: %v", err)
	}
	if gotPath != "/organizations/o1/boards" {
		t.Errorf("path = %s", gotPath)
	}
	if gotQuery.Get("filter") != "open" {
		t.Errorf("filter = %q, want open", gotQuery.Get("filter"))
	}
	if len(boards) != 1 || boards[0].ID != "b1" || boards[0].Name != "Roadmap" {
		t.Errorf("boards = %+v", boards)
	}
}
