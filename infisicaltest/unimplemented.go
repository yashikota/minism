package infisicaltest

import (
	"net/http"

	infisical "github.com/infisical/go-sdk"
	ssh "github.com/infisical/go-sdk/packages/api/ssh"
	"github.com/infisical/go-sdk/packages/models"
)

// unimplemented is the error every DynamicSecrets, Kms and Ssh call returns.
func unimplemented(op string) error {
	return apiErr(op, http.StatusNotImplemented, "minism: infisical "+op+" not implemented")
}

// ---- dynamic secrets ----

type unimplementedDynamic struct{}

func (unimplementedDynamic) List(infisical.ListDynamicSecretsRootCredentialsOptions) ([]models.DynamicSecret, error) {
	return nil, unimplemented("DynamicSecrets.List")
}
func (unimplementedDynamic) GetByName(infisical.GetDynamicSecretRootCredentialByNameOptions) (models.DynamicSecret, error) {
	return models.DynamicSecret{}, unimplemented("DynamicSecrets.GetByName")
}
func (unimplementedDynamic) Leases() infisical.DynamicSecretLeaseInterface {
	return unimplementedLeases{}
}

type unimplementedLeases struct{}

func (unimplementedLeases) List(infisical.ListDynamicSecretLeasesOptions) ([]models.DynamicSecretLease, error) {
	return nil, unimplemented("DynamicSecrets.Leases.List")
}
func (unimplementedLeases) Create(infisical.CreateDynamicSecretLeaseOptions) (map[string]any, models.DynamicSecret, models.DynamicSecretLease, error) {
	return nil, models.DynamicSecret{}, models.DynamicSecretLease{}, unimplemented("DynamicSecrets.Leases.Create")
}
func (unimplementedLeases) GetById(infisical.GetDynamicSecretLeaseByIdOptions) (models.DynamicSecretLeaseWithDynamicSecret, error) {
	return models.DynamicSecretLeaseWithDynamicSecret{}, unimplemented("DynamicSecrets.Leases.GetById")
}
func (unimplementedLeases) DeleteById(infisical.DeleteDynamicSecretLeaseOptions) (models.DynamicSecretLease, error) {
	return models.DynamicSecretLease{}, unimplemented("DynamicSecrets.Leases.DeleteById")
}
func (unimplementedLeases) RenewById(infisical.RenewDynamicSecretLeaseOptions) (models.DynamicSecretLease, error) {
	return models.DynamicSecretLease{}, unimplemented("DynamicSecrets.Leases.RenewById")
}

// ---- kms ----

type unimplementedKms struct{}

func (unimplementedKms) EncryptData(infisical.KmsEncryptDataOptions) (string, error) {
	return "", unimplemented("Kms.EncryptData")
}
func (unimplementedKms) DecryptData(infisical.KmsDecryptDataOptions) (string, error) {
	return "", unimplemented("Kms.DecryptData")
}
func (unimplementedKms) Keys() infisical.KmsKeysInterface       { return unimplementedKmsKeys{} }
func (unimplementedKms) Signing() infisical.KmsSigningInterface { return unimplementedKmsSigning{} }

type unimplementedKmsKeys struct{}

func (unimplementedKmsKeys) Create(infisical.KmsCreateKeyOptions) (infisical.KmsCreateKeyResult, error) {
	return infisical.KmsCreateKeyResult{}, unimplemented("Kms.Keys.Create")
}
func (unimplementedKmsKeys) Delete(infisical.KmsDeleteKeyOptions) (infisical.KmsDeleteKeyResult, error) {
	return infisical.KmsDeleteKeyResult{}, unimplemented("Kms.Keys.Delete")
}
func (unimplementedKmsKeys) GetByName(infisical.KmsGetKeyByNameOptions) (infisical.KmsGetKeyResult, error) {
	return infisical.KmsGetKeyResult{}, unimplemented("Kms.Keys.GetByName")
}

type unimplementedKmsSigning struct{}

func (unimplementedKmsSigning) SignData(infisical.KmsSignDataOptions) ([]byte, error) {
	return nil, unimplemented("Kms.Signing.SignData")
}
func (unimplementedKmsSigning) VerifyData(infisical.KmsVerifyDataOptions) (infisical.KmsVerifyDataResult, error) {
	return infisical.KmsVerifyDataResult{}, unimplemented("Kms.Signing.VerifyData")
}
func (unimplementedKmsSigning) ListSigningAlgorithms(infisical.KmsListSigningAlgorithmsOptions) ([]string, error) {
	return nil, unimplemented("Kms.Signing.ListSigningAlgorithms")
}
func (unimplementedKmsSigning) GetPublicKey(infisical.KmsGetPublicKeyOptions) (string, error) {
	return "", unimplemented("Kms.Signing.GetPublicKey")
}

// ---- ssh ----

type unimplementedSsh struct{}

func (unimplementedSsh) SignKey(infisical.SignSshPublicKeyOptions) (ssh.SignSshPublicKeyV1Response, error) {
	return ssh.SignSshPublicKeyV1Response{}, unimplemented("Ssh.SignKey")
}
func (unimplementedSsh) IssueCredentials(infisical.IssueSshCredsOptions) (ssh.IssueSshCredsV1Response, error) {
	return ssh.IssueSshCredsV1Response{}, unimplemented("Ssh.IssueCredentials")
}
func (unimplementedSsh) GetSshHosts(infisical.GetSshHostsOptions) (ssh.GetSshHostsV1Response, error) {
	return nil, unimplemented("Ssh.GetSshHosts")
}
func (unimplementedSsh) GetSshHostUserCaPublicKey(string) (string, error) {
	return "", unimplemented("Ssh.GetSshHostUserCaPublicKey")
}
func (unimplementedSsh) GetSshHostHostCaPublicKey(string) (string, error) {
	return "", unimplemented("Ssh.GetSshHostHostCaPublicKey")
}
func (unimplementedSsh) IssueSshHostUserCert(string, infisical.IssueSshHostUserCertOptions) (ssh.IssueSshHostUserCertV1Response, error) {
	return ssh.IssueSshHostUserCertV1Response{}, unimplemented("Ssh.IssueSshHostUserCert")
}
func (unimplementedSsh) IssueSshHostHostCert(string, infisical.IssueSshHostHostCertOptions) (ssh.IssueSshHostHostCertV1Response, error) {
	return ssh.IssueSshHostHostCertV1Response{}, unimplemented("Ssh.IssueSshHostHostCert")
}
func (unimplementedSsh) AddSshHost(infisical.AddSshHostOptions) (ssh.AddSshHostV1Response, error) {
	return ssh.AddSshHostV1Response{}, unimplemented("Ssh.AddSshHost")
}

var (
	_ infisical.DynamicSecretsInterface     = unimplementedDynamic{}
	_ infisical.DynamicSecretLeaseInterface = unimplementedLeases{}
	_ infisical.KmsInterface                = unimplementedKms{}
	_ infisical.KmsKeysInterface            = unimplementedKmsKeys{}
	_ infisical.KmsSigningInterface         = unimplementedKmsSigning{}
	_ infisical.SshInterface                = unimplementedSsh{}
)
