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
		_, _ = w.Write([]byte(`{"id":"c1","name":"Task","idList":"L1","closed":true}`))
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
	for _, f := range []string{"idList", "closed"} {
		if !strings.Contains(gotFields, f) {
			t.Errorf("fields %q missing %q", gotFields, f)
		}
	}
	if card.IDList != "L1" {
		t.Errorf("idList = %q, want L1", card.IDList)
	}
	if !card.Closed {
		t.Error("closed should decode as true")
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
