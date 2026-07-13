// Package trello is a thin client over the Trello REST API. It lets the
// assistant read the user's project boards and file new cards (tasks and bug
// reports) on them.
//
// Trello authenticates every request with an API key + user token pair, supplied
// per call (resolved from encrypted settings) rather than baked into the client,
// so the same client instance serves every request. All requests target the
// public REST API at https://api.trello.com/1.
package trello

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// base is the Trello REST API root.
const base = "https://api.trello.com/1"

// Label is a Trello card label (name may be empty for colour-only labels).
type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Card is the slice of a Trello card the assistant needs to list or report back.
type Card struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Desc     string  `json:"desc"`
	IDList   string  `json:"idList"`
	ShortURL string  `json:"shortUrl"`
	Labels   []Label `json:"labels"`
}

// List is a Trello list (a column on a board).
type List struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateCardInput describes a card to create.
type CreateCardInput struct {
	ListID   string   // required — the list the card lands on
	Name     string   // required — card title
	Desc     string   // markdown body
	LabelIDs []string // optional — labels to attach
}

// Client calls the Trello REST API. It is safe for concurrent use.
type Client struct {
	http *http.Client
}

// New creates a Trello client with a sane request timeout.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 15 * time.Second}}
}

// BoardLists returns the open lists (columns) on a board.
func (c *Client) BoardLists(ctx context.Context, apiKey, token, boardID string) ([]List, error) {
	q := url.Values{"fields": {"name"}, "filter": {"open"}}
	var lists []List
	if err := c.get(ctx, apiKey, token, "/boards/"+boardID+"/lists", q, &lists); err != nil {
		return nil, err
	}
	return lists, nil
}

// BoardCards returns the open cards on a board, each carrying its list id so the
// caller can group them by column.
func (c *Client) BoardCards(ctx context.Context, apiKey, token, boardID string) ([]Card, error) {
	q := url.Values{"fields": {"name,desc,idList,shortUrl,labels"}, "filter": {"open"}}
	var cards []Card
	if err := c.get(ctx, apiKey, token, "/boards/"+boardID+"/cards", q, &cards); err != nil {
		return nil, err
	}
	return cards, nil
}

// CreateCard creates a card and returns it (with its id and short URL).
func (c *Client) CreateCard(ctx context.Context, apiKey, token string, in CreateCardInput) (*Card, error) {
	q := url.Values{
		"idList": {in.ListID},
		"name":   {in.Name},
		"desc":   {in.Desc},
		"pos":    {"top"},
	}
	if len(in.LabelIDs) > 0 {
		q.Set("idLabels", strings.Join(in.LabelIDs, ","))
	}
	var card Card
	if err := c.post(ctx, apiKey, token, "/cards", q, &card); err != nil {
		return nil, err
	}
	return &card, nil
}

// AddChecklist creates a named checklist on a card and returns its id.
func (c *Client) AddChecklist(ctx context.Context, apiKey, token, cardID, name string) (string, error) {
	q := url.Values{"idCard": {cardID}, "name": {name}}
	var cl struct {
		ID string `json:"id"`
	}
	if err := c.post(ctx, apiKey, token, "/checklists", q, &cl); err != nil {
		return "", err
	}
	return cl.ID, nil
}

// AddCheckItem appends an item to a checklist.
func (c *Client) AddCheckItem(ctx context.Context, apiKey, token, checklistID, text string) error {
	q := url.Values{"name": {text}, "checked": {"false"}}
	return c.post(ctx, apiKey, token, "/checklists/"+checklistID+"/checkItems", q, nil)
}

// get performs an authenticated GET and decodes the JSON body into out.
func (c *Client) get(ctx context.Context, apiKey, token, path string, q url.Values, out any) error {
	return c.do(ctx, http.MethodGet, apiKey, token, path, q, out)
}

// post performs an authenticated POST (params in the query string, as Trello's
// API accepts) and decodes the JSON body into out (out may be nil to discard).
func (c *Client) post(ctx context.Context, apiKey, token, path string, q url.Values, out any) error {
	return c.do(ctx, http.MethodPost, apiKey, token, path, q, out)
}

func (c *Client) do(ctx context.Context, method, apiKey, token, path string, q url.Values, out any) error {
	if apiKey == "" || token == "" {
		return fmt.Errorf("trello is not configured")
	}
	if q == nil {
		q = url.Values{}
	}
	q.Set("key", apiKey)
	q.Set("token", token)

	req, err := http.NewRequestWithContext(ctx, method, base+path+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("trello request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("trello rejected the API key/token (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("trello rate limit reached, try again shortly")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("trello returned HTTP %d", resp.StatusCode)
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode trello response: %w", err)
	}
	return nil
}
