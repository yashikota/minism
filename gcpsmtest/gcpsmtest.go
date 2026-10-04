// Package gcpsmtest gives tests a real *secretmanager.Client talking over an
// in-memory gRPC connection (bufconn) to a small SecretManagerServiceServer.
//
// The server interface and every message type come from Google's generated
// secretmanagerpb package; only the state behind the RPCs is ours.
package gcpsmtest

import (
	"context"
	"hash/crc32"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

type ver struct {
	num     int
	data    []byte
	state   secretmanagerpb.SecretVersion_State
	created time.Time
}

type secretState struct {
	name     string // projects/p/secrets/id
	labels   map[string]string
	created  time.Time
	versions []*ver
}

// Server is an in-memory Secret Manager.
type Server struct {
	secretmanagerpb.UnimplementedSecretManagerServiceServer

	t       testing.TB
	lis     *bufconn.Listener
	mu      sync.Mutex
	secrets map[string]*secretState
}

// New starts the in-memory server; it is stopped by t.Cleanup.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{t: t, lis: bufconn.Listen(1 << 20), secrets: map[string]*secretState{}}
	gs := grpc.NewServer()
	secretmanagerpb.RegisterSecretManagerServiceServer(gs, s)
	go func() { _ = gs.Serve(s.lis) }()
	t.Cleanup(gs.Stop)
	return s
}

// Client returns a real secretmanager.Client connected to this Server.
func (s *Server) Client() *secretmanager.Client {
	s.t.Helper()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return s.lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		s.t.Fatalf("gcpsmtest: dial: %v", err)
	}
	s.t.Cleanup(func() { _ = conn.Close() })
	c, err := secretmanager.NewClient(context.Background(), option.WithGRPCConn(conn))
	if err != nil {
		s.t.Fatalf("gcpsmtest: new client: %v", err)
	}
	s.t.Cleanup(func() { _ = c.Close() })
	return c
}

// Value returns the payload of the latest enabled version (fixture inspection).
func (s *Server) Value(secret string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.secrets[secret]
	if !ok {
		return nil, false
	}
	for i := len(sec.versions) - 1; i >= 0; i-- {
		if v := sec.versions[i]; v.state == secretmanagerpb.SecretVersion_ENABLED {
			return v.data, true
		}
	}
	return nil, false
}

func (sec *secretState) proto() *secretmanagerpb.Secret {
	return &secretmanagerpb.Secret{
		Name:        sec.name,
		CreateTime:  timestamppb.New(sec.created),
		Labels:      sec.labels,
		Replication: &secretmanagerpb.Replication{Replication: &secretmanagerpb.Replication_Automatic_{Automatic: &secretmanagerpb.Replication_Automatic{}}},
	}
}

func (sec *secretState) versionProto(v *ver) *secretmanagerpb.SecretVersion {
	return &secretmanagerpb.SecretVersion{
		Name:       sec.name + "/versions/" + strconv.Itoa(v.num),
		CreateTime: timestamppb.New(v.created),
		State:      v.state,
	}
}

// resolve finds a secret and version from "projects/p/secrets/s/versions/{n|latest}".
func (s *Server) resolve(name string) (*secretState, *ver, error) {
	i := strings.Index(name, "/versions/")
	if i < 0 {
		return nil, nil, status.Errorf(codes.InvalidArgument, "invalid secret version name %q", name)
	}
	sec, ok := s.secrets[name[:i]]
	if !ok {
		return nil, nil, status.Errorf(codes.NotFound, "Secret [%s] not found.", name[:i])
	}
	id := name[i+len("/versions/"):]
	if id == "latest" {
		if len(sec.versions) == 0 {
			return nil, nil, status.Errorf(codes.NotFound, "Secret Version [%s] not found.", name)
		}
		return sec, sec.versions[len(sec.versions)-1], nil
	}
	n, err := strconv.Atoi(id)
	if err != nil || n < 1 || n > len(sec.versions) {
		return nil, nil, status.Errorf(codes.NotFound, "Secret Version [%s] not found.", name)
	}
	return sec, sec.versions[n-1], nil
}

func (s *Server) secret(name string) (*secretState, error) {
	sec, ok := s.secrets[name]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "Secret [%s] not found.", name)
	}
	return sec, nil
}

func (s *Server) CreateSecret(_ context.Context, req *secretmanagerpb.CreateSecretRequest) (*secretmanagerpb.Secret, error) {
	if req.GetSecretId() == "" || req.GetParent() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent and secret_id are required")
	}
	name := req.GetParent() + "/secrets/" + req.GetSecretId()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.secrets[name]; ok {
		return nil, status.Errorf(codes.AlreadyExists, "Secret [%s] already exists.", name)
	}
	sec := &secretState{name: name, labels: req.GetSecret().GetLabels(), created: time.Now().UTC()}
	s.secrets[name] = sec
	return sec.proto(), nil
}

func (s *Server) GetSecret(_ context.Context, req *secretmanagerpb.GetSecretRequest) (*secretmanagerpb.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, err := s.secret(req.GetName())
	if err != nil {
		return nil, err
	}
	return sec.proto(), nil
}

func (s *Server) UpdateSecret(_ context.Context, req *secretmanagerpb.UpdateSecretRequest) (*secretmanagerpb.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, err := s.secret(req.GetSecret().GetName())
	if err != nil {
		return nil, err
	}
	for _, p := range req.GetUpdateMask().GetPaths() {
		if p == "labels" {
			sec.labels = req.GetSecret().GetLabels()
		}
	}
	return sec.proto(), nil
}

func (s *Server) ListSecrets(_ context.Context, req *secretmanagerpb.ListSecretsRequest) (*secretmanagerpb.ListSecretsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var names []string
	for n := range s.secrets {
		if strings.HasPrefix(n, req.GetParent()+"/secrets/") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	resp := &secretmanagerpb.ListSecretsResponse{TotalSize: int32(len(names))}
	for _, n := range names {
		resp.Secrets = append(resp.Secrets, s.secrets[n].proto())
	}
	return resp, nil
}

func (s *Server) DeleteSecret(_ context.Context, req *secretmanagerpb.DeleteSecretRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.secret(req.GetName()); err != nil {
		return nil, err
	}
	delete(s.secrets, req.GetName())
	return &emptypb.Empty{}, nil
}

func (s *Server) AddSecretVersion(_ context.Context, req *secretmanagerpb.AddSecretVersionRequest) (*secretmanagerpb.SecretVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, err := s.secret(req.GetParent())
	if err != nil {
		return nil, err
	}
	data := req.GetPayload().GetData()
	if c := req.GetPayload().DataCrc32C; c != nil && *c != int64(crc32.Checksum(data, castagnoli)) {
		return nil, status.Error(codes.InvalidArgument, "Checksum verification failed: data_crc32c does not match payload")
	}
	v := &ver{num: len(sec.versions) + 1, data: append([]byte(nil), data...), state: secretmanagerpb.SecretVersion_ENABLED, created: time.Now().UTC()}
	sec.versions = append(sec.versions, v)
	return sec.versionProto(v), nil
}

func (s *Server) GetSecretVersion(_ context.Context, req *secretmanagerpb.GetSecretVersionRequest) (*secretmanagerpb.SecretVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, v, err := s.resolve(req.GetName())
	if err != nil {
		return nil, err
	}
	return sec.versionProto(v), nil
}

func (s *Server) AccessSecretVersion(_ context.Context, req *secretmanagerpb.AccessSecretVersionRequest) (*secretmanagerpb.AccessSecretVersionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, v, err := s.resolve(req.GetName())
	if err != nil {
		return nil, err
	}
	if v.state != secretmanagerpb.SecretVersion_ENABLED {
		return nil, status.Errorf(codes.FailedPrecondition, "Secret Version [%s] is in %s state.", sec.versionProto(v).Name, v.state)
	}
	crc := int64(crc32.Checksum(v.data, castagnoli))
	return &secretmanagerpb.AccessSecretVersionResponse{
		Name:    sec.versionProto(v).Name,
		Payload: &secretmanagerpb.SecretPayload{Data: v.data, DataCrc32C: &crc},
	}, nil
}

func (s *Server) ListSecretVersions(_ context.Context, req *secretmanagerpb.ListSecretVersionsRequest) (*secretmanagerpb.ListSecretVersionsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, err := s.secret(req.GetParent())
	if err != nil {
		return nil, err
	}
	resp := &secretmanagerpb.ListSecretVersionsResponse{TotalSize: int32(len(sec.versions))}
	for i := len(sec.versions) - 1; i >= 0; i-- { // newest first, like the real API
		resp.Versions = append(resp.Versions, sec.versionProto(sec.versions[i]))
	}
	return resp, nil
}

func (s *Server) setState(name string, from func(secretmanagerpb.SecretVersion_State) bool, to secretmanagerpb.SecretVersion_State) (*secretmanagerpb.SecretVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, v, err := s.resolve(name)
	if err != nil {
		return nil, err
	}
	if !from(v.state) {
		return nil, status.Errorf(codes.FailedPrecondition, "Secret Version [%s] is in %s state.", sec.versionProto(v).Name, v.state)
	}
	v.state = to
	if to == secretmanagerpb.SecretVersion_DESTROYED {
		v.data = nil
	}
	return sec.versionProto(v), nil
}

func (s *Server) DisableSecretVersion(_ context.Context, req *secretmanagerpb.DisableSecretVersionRequest) (*secretmanagerpb.SecretVersion, error) {
	return s.setState(req.GetName(), func(st secretmanagerpb.SecretVersion_State) bool {
		return st != secretmanagerpb.SecretVersion_DESTROYED
	}, secretmanagerpb.SecretVersion_DISABLED)
}

func (s *Server) EnableSecretVersion(_ context.Context, req *secretmanagerpb.EnableSecretVersionRequest) (*secretmanagerpb.SecretVersion, error) {
	return s.setState(req.GetName(), func(st secretmanagerpb.SecretVersion_State) bool {
		return st != secretmanagerpb.SecretVersion_DESTROYED
	}, secretmanagerpb.SecretVersion_ENABLED)
}

func (s *Server) DestroySecretVersion(_ context.Context, req *secretmanagerpb.DestroySecretVersionRequest) (*secretmanagerpb.SecretVersion, error) {
	return s.setState(req.GetName(), func(st secretmanagerpb.SecretVersion_State) bool {
		return st != secretmanagerpb.SecretVersion_DESTROYED
	}, secretmanagerpb.SecretVersion_DESTROYED)
}
