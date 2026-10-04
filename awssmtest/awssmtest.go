// Package awssmtest gives tests a real *secretsmanager.Client whose HTTP layer
// is an in-memory Secrets Manager. The client's own serializer, signer and
// middleware all run; only the far end of the wire is replaced.
//
// AWS publishes no server-side implementation, so the state machine here is
// ours, limited to what tests need: no rotation, IAM, KMS or replication.
package awssmtest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/yashikota/minism/internal/rtfake"
)

const (
	region  = "us-east-1"
	account = "123456789012"
	current = "AWSCURRENT"
	prev    = "AWSPREVIOUS"
)

type version struct {
	id      string
	str     *string
	bin     []byte
	stages  []string
	created time.Time
}

type secret struct {
	name, arn, desc string
	tags            []tag
	versions        []*version // oldest first
	created         time.Time
	changed         time.Time
	deleted         *time.Time
}

type tag struct{ Key, Value string }

// Server is an in-memory Secrets Manager.
type Server struct {
	t       testing.TB
	mu      sync.Mutex
	secrets map[string]*secret // by name
	seq     int
	now     func() time.Time
}

// New returns an empty Server.
func New(t testing.TB) *Server {
	t.Helper()
	return &Server{t: t, secrets: map[string]*secret{}, now: func() time.Time { return time.Now().UTC() }}
}

// Client returns a real secretsmanager.Client talking to this Server.
func (s *Server) Client() *secretsmanager.Client {
	return secretsmanager.NewFromConfig(aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider("AKIAMINISM", "minism", ""),
		HTTPClient:  rtfake.Client(http.HandlerFunc(s.serve)),
		Retryer:     func() aws.Retryer { return aws.NopRetryer{} },
	})
}

// Value returns the current string value of a secret (fixture inspection).
func (s *Server) Value(name string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.secrets[name]
	if !ok || sec.deleted != nil {
		return "", false
	}
	v := sec.byStage(current)
	if v == nil || v.str == nil {
		return "", false
	}
	return *v.str, true
}

func (sec *secret) byStage(stage string) *version {
	for _, v := range sec.versions {
		for _, st := range v.stages {
			if st == stage {
				return v
			}
		}
	}
	return nil
}

func (sec *secret) byID(id string) *version {
	for _, v := range sec.versions {
		if v.id == id {
			return v
		}
	}
	return nil
}

// addVersion makes v the AWSCURRENT version and demotes the previous one.
func (sec *secret) addVersion(v *version, now time.Time) {
	for _, old := range sec.versions {
		var keep []string
		for _, st := range old.stages {
			switch st {
			case current:
				keep = append(keep, prev)
			case prev: // dropped: only one AWSPREVIOUS
			default:
				keep = append(keep, st)
			}
		}
		old.stages = keep
	}
	v.stages = append([]string{current}, v.stages...)
	sec.versions = append(sec.versions, v)
	sec.changed = now
}

func (s *Server) lookup(id string) (*secret, bool) {
	if sec, ok := s.secrets[id]; ok {
		return sec, true
	}
	for _, sec := range s.secrets {
		if sec.arn == id {
			return sec, true
		}
	}
	return nil, false
}

func (s *Server) nextToken() string {
	s.seq++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", s.seq)
}

func epoch(t time.Time) float64 { return float64(t.UnixNano()) / 1e9 }

type apiError struct {
	status int
	typ    string
	msg    string
}

func (e *apiError) Error() string { return e.typ + ": " + e.msg }

func notFound() *apiError {
	return &apiError{400, "ResourceNotFoundException", "Secrets Manager can't find the specified secret."}
}

func invalid(msg string) *apiError { return &apiError{400, "InvalidRequestException", msg} }

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	op := strings.TrimPrefix(r.Header.Get("X-Amz-Target"), "secretsmanager.")
	var in map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, &apiError{400, "SerializationException", err.Error()})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var (
		out any
		err *apiError
	)
	switch op {
	case "CreateSecret":
		out, err = s.createSecret(in)
	case "PutSecretValue":
		out, err = s.putSecretValue(in)
	case "UpdateSecret":
		out, err = s.updateSecret(in)
	case "GetSecretValue":
		out, err = s.getSecretValue(in)
	case "DescribeSecret":
		out, err = s.describeSecret(in)
	case "ListSecrets":
		out, err = s.listSecrets()
	case "DeleteSecret":
		out, err = s.deleteSecret(in)
	case "RestoreSecret":
		out, err = s.restoreSecret(in)
	default:
		err = &apiError{400, "UnknownOperationException", fmt.Sprintf("minism: awssm %s not implemented", op)}
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	rtfake.JSON(w, http.StatusOK, "application/x-amz-json-1.1", out)
}

func writeErr(w http.ResponseWriter, e *apiError) {
	w.Header().Set("X-Amzn-Errortype", e.typ)
	rtfake.JSON(w, e.status, "application/x-amz-json-1.1", map[string]string{"__type": e.typ, "Message": e.msg})
}

// field decodes one request member, ignoring absent ones.
func field[T any](in map[string]json.RawMessage, key string) (v T, ok bool) {
	raw, present := in[key]
	if !present || string(raw) == "null" {
		return v, false
	}
	return v, json.Unmarshal(raw, &v) == nil
}

func newVersion(in map[string]json.RawMessage, id string, now time.Time) (*version, *apiError) {
	v := &version{id: id, created: now}
	if str, ok := field[string](in, "SecretString"); ok {
		v.str = &str
	}
	if b64, ok := field[string](in, "SecretBinary"); ok {
		bin, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, &apiError{400, "InvalidParameterException", "SecretBinary is not valid base64"}
		}
		v.bin = bin
	}
	if stages, ok := field[[]string](in, "VersionStages"); ok {
		for _, st := range stages {
			if st != current {
				v.stages = append(v.stages, st)
			}
		}
	}
	return v, nil
}

func (s *Server) createSecret(in map[string]json.RawMessage) (any, *apiError) {
	name, _ := field[string](in, "Name")
	if name == "" {
		return nil, &apiError{400, "InvalidParameterException", "Name is required"}
	}
	if old, ok := s.secrets[name]; ok {
		if old.deleted != nil {
			return nil, invalid("You can't create this secret because a secret with this name is already scheduled for deletion.")
		}
		return nil, &apiError{400, "ResourceExistsException", "The operation failed because the secret " + name + " already exists."}
	}
	now := s.now()
	token, ok := field[string](in, "ClientRequestToken")
	if !ok {
		token = s.nextToken()
	}
	sec := &secret{name: name, created: now, changed: now}
	sec.desc, _ = field[string](in, "Description")
	sec.tags, _ = field[[]tag](in, "Tags")
	s.seq++
	sec.arn = fmt.Sprintf("arn:aws:secretsmanager:%s:%s:secret:%s-%06d", region, account, name, s.seq)
	_, hasStr := in["SecretString"]
	_, hasBin := in["SecretBinary"]
	resp := map[string]any{"ARN": sec.arn, "Name": name}
	if hasStr || hasBin {
		v, err := newVersion(in, token, now)
		if err != nil {
			return nil, err
		}
		sec.addVersion(v, now)
		resp["VersionId"] = v.id
	}
	s.secrets[name] = sec
	return resp, nil
}

func (s *Server) writable(in map[string]json.RawMessage) (*secret, *apiError) {
	id, _ := field[string](in, "SecretId")
	sec, ok := s.lookup(id)
	if !ok {
		return nil, notFound()
	}
	if sec.deleted != nil {
		return nil, invalid("You can't perform this operation on the secret because it was marked for deletion.")
	}
	return sec, nil
}

func (s *Server) putSecretValue(in map[string]json.RawMessage) (any, *apiError) {
	sec, err := s.writable(in)
	if err != nil {
		return nil, err
	}
	now := s.now()
	token, ok := field[string](in, "ClientRequestToken")
	if !ok {
		token = s.nextToken()
	}
	v, err := newVersion(in, token, now)
	if err != nil {
		return nil, err
	}
	sec.addVersion(v, now)
	return map[string]any{"ARN": sec.arn, "Name": sec.name, "VersionId": v.id, "VersionStages": v.stages}, nil
}

func (s *Server) updateSecret(in map[string]json.RawMessage) (any, *apiError) {
	sec, err := s.writable(in)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if d, ok := field[string](in, "Description"); ok {
		sec.desc = d
		sec.changed = now
	}
	resp := map[string]any{"ARN": sec.arn, "Name": sec.name}
	_, hasStr := in["SecretString"]
	_, hasBin := in["SecretBinary"]
	if hasStr || hasBin {
		token, ok := field[string](in, "ClientRequestToken")
		if !ok {
			token = s.nextToken()
		}
		v, err := newVersion(in, token, now)
		if err != nil {
			return nil, err
		}
		sec.addVersion(v, now)
		resp["VersionId"] = v.id
	}
	return resp, nil
}

func (s *Server) getSecretValue(in map[string]json.RawMessage) (any, *apiError) {
	sec, err := s.writable(in)
	if err != nil {
		return nil, err
	}
	var v *version
	if id, ok := field[string](in, "VersionId"); ok {
		v = sec.byID(id)
	} else {
		stage, ok := field[string](in, "VersionStage")
		if !ok {
			stage = current
		}
		v = sec.byStage(stage)
	}
	if v == nil {
		return nil, notFound()
	}
	resp := map[string]any{
		"ARN": sec.arn, "Name": sec.name, "VersionId": v.id,
		"VersionStages": v.stages, "CreatedDate": epoch(v.created),
	}
	if v.str != nil {
		resp["SecretString"] = *v.str
	}
	if v.bin != nil {
		resp["SecretBinary"] = base64.StdEncoding.EncodeToString(v.bin)
	}
	return resp, nil
}

func (sec *secret) describe() map[string]any {
	stages := map[string][]string{}
	for _, v := range sec.versions {
		stages[v.id] = v.stages
	}
	d := map[string]any{
		"ARN": sec.arn, "Name": sec.name, "Description": sec.desc, "Tags": sec.tags,
		"VersionIdsToStages": stages, "CreatedDate": epoch(sec.created), "LastChangedDate": epoch(sec.changed),
	}
	if sec.deleted != nil {
		d["DeletedDate"] = epoch(*sec.deleted)
	}
	if sec.tags == nil {
		d["Tags"] = []tag{}
	}
	return d
}

func (s *Server) describeSecret(in map[string]json.RawMessage) (any, *apiError) {
	id, _ := field[string](in, "SecretId")
	sec, ok := s.lookup(id)
	if !ok {
		return nil, notFound()
	}
	return sec.describe(), nil
}

func (s *Server) listSecrets() (any, *apiError) {
	names := make([]string, 0, len(s.secrets))
	for n, sec := range s.secrets {
		if sec.deleted == nil {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	list := make([]map[string]any, 0, len(names))
	for _, n := range names {
		list = append(list, s.secrets[n].describe())
	}
	return map[string]any{"SecretList": list}, nil
}

func (s *Server) deleteSecret(in map[string]json.RawMessage) (any, *apiError) {
	id, _ := field[string](in, "SecretId")
	sec, ok := s.lookup(id)
	if !ok {
		return nil, notFound()
	}
	now := s.now()
	if force, _ := field[bool](in, "ForceDeleteWithoutRecovery"); force {
		delete(s.secrets, sec.name)
	} else {
		days, ok := field[int](in, "RecoveryWindowInDays")
		if !ok {
			days = 30
		}
		when := now.AddDate(0, 0, days)
		sec.deleted = &when
	}
	return map[string]any{"ARN": sec.arn, "Name": sec.name, "DeletionDate": epoch(now)}, nil
}

func (s *Server) restoreSecret(in map[string]json.RawMessage) (any, *apiError) {
	id, _ := field[string](in, "SecretId")
	sec, ok := s.lookup(id)
	if !ok {
		return nil, notFound()
	}
	sec.deleted = nil
	return map[string]any{"ARN": sec.arn, "Name": sec.name}, nil
}
