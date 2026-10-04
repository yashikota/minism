// Package onepasswordtest gives tests a real *onepassword.Client whose
// ItemsAPI, VaultsAPI and SecretsAPI are in-memory.
//
// onepassword.Client exposes those APIs as exported fields, so the real
// Client type is used as-is and the WASM core / network are never involved.
package onepasswordtest

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1password/onepassword-sdk-go"
)

// ErrNotFound is wrapped by errors for missing vaults, items and fields.
var ErrNotFound = errors.New("not found")

var errUnimplemented = errors.New("minism: 1password operation not implemented")

type vault struct {
	onepassword.Vault
	created, updated time.Time
	items            map[string]*onepassword.Item
	archived         map[string]bool
}

// Server is an in-memory 1Password account.
type Server struct {
	mu     sync.Mutex
	seq    int
	vaults map[string]*vault
	now    func() time.Time
}

// New returns an empty account.
func New(t testing.TB) *Server {
	t.Helper()
	return &Server{vaults: map[string]*vault{}, now: func() time.Time { return time.Now().UTC() }}
}

// Client returns a real onepassword.Client backed by this account.
func (s *Server) Client() *onepassword.Client {
	return &onepassword.Client{
		ItemsAPI:   &items{s},
		VaultsAPI:  &vaults{s},
		SecretsAPI: &secrets{s},
	}
}

// AddVault is a fixture helper returning the new vault's ID.
func (s *Server) AddVault(title string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.newVault(title, "").ID
}

func (s *Server) id() string {
	s.seq++
	return fmt.Sprintf("%026d", s.seq)
}

func (s *Server) newVault(title, desc string) *vault {
	now := s.now()
	v := &vault{
		Vault:   onepassword.Vault{ID: s.id(), Title: title, Description: desc, VaultType: onepassword.VaultTypeUserCreated},
		created: now, updated: now,
		items: map[string]*onepassword.Item{}, archived: map[string]bool{},
	}
	s.vaults[v.ID] = v
	return v
}

func (s *Server) vault(id string) (*vault, error) {
	if v, ok := s.vaults[id]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("vault %q: %w", id, ErrNotFound)
}

func (s *Server) vaultByRef(ref string) (*vault, error) {
	if v, ok := s.vaults[ref]; ok {
		return v, nil
	}
	for _, v := range s.vaults {
		if v.Title == ref {
			return v, nil
		}
	}
	return nil, fmt.Errorf("vault %q: %w", ref, ErrNotFound)
}

func (v *vault) item(id string) (*onepassword.Item, error) {
	if it, ok := v.items[id]; ok {
		return it, nil
	}
	return nil, fmt.Errorf("item %q in vault %q: %w", id, v.ID, ErrNotFound)
}

func (v *vault) itemByRef(ref string) (*onepassword.Item, error) {
	if it, ok := v.items[ref]; ok && !v.archived[ref] {
		return it, nil
	}
	for id, it := range v.items {
		if it.Title == ref && !v.archived[id] {
			return it, nil
		}
	}
	return nil, fmt.Errorf("item %q in vault %q: %w", ref, v.Title, ErrNotFound)
}

func clone(it onepassword.Item) onepassword.Item {
	it.Fields = append([]onepassword.ItemField(nil), it.Fields...)
	it.Sections = append([]onepassword.ItemSection(nil), it.Sections...)
	it.Tags = append([]string(nil), it.Tags...)
	return it
}

func overview(v *vault, it *onepassword.Item) onepassword.ItemOverview {
	state := onepassword.ItemStateActive
	if v.archived[it.ID] {
		state = onepassword.ItemStateArchived
	}
	return onepassword.ItemOverview{
		ID: it.ID, Title: it.Title, Category: it.Category, VaultID: it.VaultID,
		Websites: it.Websites, Tags: it.Tags, CreatedAt: it.CreatedAt, UpdatedAt: it.UpdatedAt, State: state,
	}
}

// ---- ItemsAPI ----

type items struct{ s *Server }

func (a *items) create(p onepassword.ItemCreateParams) (onepassword.Item, error) {
	v, err := a.s.vault(p.VaultID)
	if err != nil {
		return onepassword.Item{}, err
	}
	now := a.s.now()
	it := &onepassword.Item{
		ID: a.s.id(), Title: p.Title, Category: p.Category, VaultID: v.ID, Fields: p.Fields,
		Sections: p.Sections, Tags: p.Tags, Websites: p.Websites, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if p.Notes != nil {
		it.Notes = *p.Notes
	}
	for i := range it.Fields {
		if it.Fields[i].ID == "" {
			it.Fields[i].ID = strings.ToLower(it.Fields[i].Title)
		}
	}
	v.items[it.ID] = it
	v.ActiveItemCount++
	return clone(*it), nil
}

func (a *items) Create(_ context.Context, p onepassword.ItemCreateParams) (onepassword.Item, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	return a.create(p)
}

func (a *items) CreateAll(_ context.Context, vaultID string, ps []onepassword.ItemCreateParams) (onepassword.ItemsUpdateAllResponse, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	var resp onepassword.ItemsUpdateAllResponse
	for _, p := range ps {
		p.VaultID = vaultID
		it, err := a.create(p)
		if err != nil {
			resp.IndividualResponses = append(resp.IndividualResponses, onepassword.Response[onepassword.Item, onepassword.ItemUpdateFailureReason]{
				Error: &onepassword.ItemUpdateFailureReason{Type: onepassword.ItemUpdateFailureReasonTypeVariantItemValidationError},
			})
			continue
		}
		resp.IndividualResponses = append(resp.IndividualResponses, onepassword.Response[onepassword.Item, onepassword.ItemUpdateFailureReason]{Content: &it})
	}
	return resp, nil
}

func (a *items) Get(_ context.Context, vaultID, itemID string) (onepassword.Item, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	v, err := a.s.vault(vaultID)
	if err != nil {
		return onepassword.Item{}, err
	}
	it, err := v.item(itemID)
	if err != nil {
		return onepassword.Item{}, err
	}
	return clone(*it), nil
}

func (a *items) GetAll(ctx context.Context, vaultID string, ids []string) (onepassword.ItemsGetAllResponse, error) {
	var resp onepassword.ItemsGetAllResponse
	for _, id := range ids {
		it, err := a.Get(ctx, vaultID, id)
		if err != nil {
			resp.IndividualResponses = append(resp.IndividualResponses, onepassword.Response[onepassword.Item, onepassword.ItemsGetAllError]{
				Error: &onepassword.ItemsGetAllError{Type: onepassword.ItemsGetAllErrorTypeVariantItemNotFound},
			})
			continue
		}
		resp.IndividualResponses = append(resp.IndividualResponses, onepassword.Response[onepassword.Item, onepassword.ItemsGetAllError]{Content: &it})
	}
	return resp, nil
}

func (a *items) Put(_ context.Context, item onepassword.Item) (onepassword.Item, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	v, err := a.s.vault(item.VaultID)
	if err != nil {
		return onepassword.Item{}, err
	}
	cur, err := v.item(item.ID)
	if err != nil {
		return onepassword.Item{}, err
	}
	if item.Version != cur.Version {
		return onepassword.Item{}, fmt.Errorf("item %q: incorrect item version (have %d, current %d)", item.ID, item.Version, cur.Version)
	}
	item.Version++
	item.CreatedAt = cur.CreatedAt
	item.UpdatedAt = a.s.now()
	stored := clone(item)
	v.items[item.ID] = &stored
	return clone(stored), nil
}

func (a *items) Delete(_ context.Context, vaultID, itemID string) error {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	v, err := a.s.vault(vaultID)
	if err != nil {
		return err
	}
	if _, err := v.item(itemID); err != nil {
		return err
	}
	delete(v.items, itemID)
	delete(v.archived, itemID)
	v.ActiveItemCount--
	return nil
}

func (a *items) DeleteAll(ctx context.Context, vaultID string, ids []string) (onepassword.ItemsDeleteAllResponse, error) {
	resp := onepassword.ItemsDeleteAllResponse{IndividualResponses: map[string]onepassword.Response[struct{}, onepassword.ItemUpdateFailureReason]{}}
	for _, id := range ids {
		if err := a.Delete(ctx, vaultID, id); err != nil {
			resp.IndividualResponses[id] = onepassword.Response[struct{}, onepassword.ItemUpdateFailureReason]{
				Error: &onepassword.ItemUpdateFailureReason{Type: onepassword.ItemUpdateFailureReasonTypeVariantItemNotFound},
			}
			continue
		}
		resp.IndividualResponses[id] = onepassword.Response[struct{}, onepassword.ItemUpdateFailureReason]{Content: &struct{}{}}
	}
	return resp, nil
}

func (a *items) Archive(_ context.Context, vaultID, itemID string) error {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	v, err := a.s.vault(vaultID)
	if err != nil {
		return err
	}
	if _, err := v.item(itemID); err != nil {
		return err
	}
	if !v.archived[itemID] {
		v.archived[itemID] = true
		v.ActiveItemCount--
	}
	return nil
}

func (a *items) List(_ context.Context, vaultID string, _ ...onepassword.ItemListFilter) ([]onepassword.ItemOverview, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	v, err := a.s.vault(vaultID)
	if err != nil {
		return nil, err
	}
	out := []onepassword.ItemOverview{}
	for id, it := range v.items {
		if !v.archived[id] { // the real default filter lists active items only
			out = append(out, overview(v, it))
		}
	}
	sortOverviews(out)
	return out, nil
}

func (a *items) Shares() onepassword.ItemsSharesAPI { return sharesStub{} }
func (a *items) Files() onepassword.ItemsFilesAPI   { return filesStub{} }

type sharesStub struct{}

func (sharesStub) GetAccountPolicy(context.Context, string, string) (onepassword.ItemShareAccountPolicy, error) {
	return onepassword.ItemShareAccountPolicy{}, errUnimplemented
}

func (sharesStub) ValidateRecipients(context.Context, onepassword.ItemShareAccountPolicy, []string) ([]onepassword.ValidRecipient, error) {
	return nil, errUnimplemented
}

func (sharesStub) Create(context.Context, onepassword.Item, onepassword.ItemShareAccountPolicy, onepassword.ItemShareParams) (string, error) {
	return "", errUnimplemented
}

type filesStub struct{}

func (filesStub) Attach(context.Context, onepassword.Item, onepassword.FileCreateParams) (onepassword.Item, error) {
	return onepassword.Item{}, errUnimplemented
}

func (filesStub) Read(context.Context, string, string, onepassword.FileAttributes) ([]byte, error) {
	return nil, errUnimplemented
}

func (filesStub) Delete(context.Context, onepassword.Item, string, string) (onepassword.Item, error) {
	return onepassword.Item{}, errUnimplemented
}

func (filesStub) ReplaceDocument(context.Context, onepassword.Item, onepassword.DocumentCreateParams) (onepassword.Item, error) {
	return onepassword.Item{}, errUnimplemented
}

// ---- VaultsAPI ----

type vaults struct{ s *Server }

func (a *vaults) Create(_ context.Context, p onepassword.VaultCreateParams) (onepassword.Vault, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	desc := ""
	if p.Description != nil {
		desc = *p.Description
	}
	return a.s.newVault(p.Title, desc).Vault, nil
}

func (a *vaults) List(context.Context, ...onepassword.VaultListParams) ([]onepassword.VaultOverview, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	out := []onepassword.VaultOverview{}
	for _, v := range a.s.vaults {
		out = append(out, vaultOverview(v))
	}
	sortVaults(out)
	return out, nil
}

func vaultOverview(v *vault) onepassword.VaultOverview {
	return onepassword.VaultOverview{
		ID: v.ID, Title: v.Title, Description: v.Description, VaultType: v.VaultType,
		ActiveItemCount: v.ActiveItemCount, ContentVersion: v.ContentVersion, AttributeVersion: v.AttributeVersion,
		CreatedAt: v.created, UpdatedAt: v.updated,
	}
}

func (a *vaults) GetOverview(_ context.Context, id string) (onepassword.VaultOverview, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	v, err := a.s.vault(id)
	if err != nil {
		return onepassword.VaultOverview{}, err
	}
	return vaultOverview(v), nil
}

func (a *vaults) Get(_ context.Context, id string, _ onepassword.VaultGetParams) (onepassword.Vault, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	v, err := a.s.vault(id)
	if err != nil {
		return onepassword.Vault{}, err
	}
	return v.Vault, nil
}

func (a *vaults) Update(_ context.Context, id string, p onepassword.VaultUpdateParams) (onepassword.Vault, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	v, err := a.s.vault(id)
	if err != nil {
		return onepassword.Vault{}, err
	}
	if p.Title != nil {
		v.Title = *p.Title
	}
	if p.Description != nil {
		v.Description = *p.Description
	}
	v.AttributeVersion++
	v.updated = a.s.now()
	return v.Vault, nil
}

func (a *vaults) Delete(_ context.Context, id string) error {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	if _, err := a.s.vault(id); err != nil {
		return err
	}
	delete(a.s.vaults, id)
	return nil
}

func (*vaults) GrantGroupPermissions(context.Context, string, []onepassword.GroupAccess) error {
	return errUnimplemented
}

func (*vaults) UpdateGroupPermissions(context.Context, []onepassword.GroupVaultAccess) error {
	return errUnimplemented
}

func (*vaults) RevokeGroupPermissions(context.Context, string, string) error { return errUnimplemented }

// ---- SecretsAPI ----

type secrets struct{ s *Server }

// resolve implements op://vault/item[/section]/field[?attribute=...] lookup.
func (a *secrets) resolve(ref string) (string, onepassword.ResolveReferenceErrorTypes, error) {
	u, err := url.Parse(ref)
	if err != nil || u.Scheme != "op" {
		return "", onepassword.ResolveReferenceErrorTypeVariantParsing, fmt.Errorf("invalid secret reference %q", ref)
	}
	parts := strings.Split(strings.Trim(u.Host+u.Path, "/"), "/")
	if len(parts) < 3 || len(parts) > 4 {
		return "", onepassword.ResolveReferenceErrorTypeVariantParsing, fmt.Errorf("invalid secret reference %q", ref)
	}
	v, err := a.s.vaultByRef(parts[0])
	if err != nil {
		return "", onepassword.ResolveReferenceErrorTypeVariantVaultNotFound, err
	}
	it, err := v.itemByRef(parts[1])
	if err != nil {
		return "", onepassword.ResolveReferenceErrorTypeVariantItemNotFound, err
	}
	field := parts[len(parts)-1]
	section := ""
	if len(parts) == 4 {
		section = parts[2]
	}
	for _, f := range it.Fields {
		if f.ID != field && f.Title != field {
			continue
		}
		if section != "" && !inSection(it, f, section) {
			continue
		}
		return f.Value, "", nil
	}
	return "", onepassword.ResolveReferenceErrorTypeVariantFieldNotFound, fmt.Errorf("field %q in item %q: %w", field, it.Title, ErrNotFound)
}

func inSection(it *onepassword.Item, f onepassword.ItemField, section string) bool {
	if f.SectionID == nil {
		return false
	}
	for _, sec := range it.Sections {
		if sec.ID == *f.SectionID && (sec.ID == section || sec.Title == section) {
			return true
		}
	}
	return false
}

func (a *secrets) Resolve(_ context.Context, ref string) (string, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	val, _, err := a.resolve(ref)
	if err != nil {
		return "", fmt.Errorf("error resolving secret reference: %w", err)
	}
	return val, nil
}

func (a *secrets) ResolveAll(_ context.Context, refs []string) (onepassword.ResolveAllResponse, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	resp := onepassword.ResolveAllResponse{IndividualResponses: map[string]onepassword.Response[onepassword.ResolvedReference, onepassword.ResolveReferenceError]{}}
	for _, ref := range refs {
		val, kind, err := a.resolve(ref)
		if err != nil {
			resp.IndividualResponses[ref] = onepassword.Response[onepassword.ResolvedReference, onepassword.ResolveReferenceError]{
				Error: &onepassword.ResolveReferenceError{Type: kind},
			}
			continue
		}
		resp.IndividualResponses[ref] = onepassword.Response[onepassword.ResolvedReference, onepassword.ResolveReferenceError]{
			Content: &onepassword.ResolvedReference{Secret: val},
		}
	}
	return resp, nil
}
