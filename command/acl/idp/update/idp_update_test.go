package idpupdate

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/consul/agent"
	"github.com/hashicorp/consul/agent/connect"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/logger"
	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/hashicorp/consul/testrpc"
	"github.com/hashicorp/go-uuid"
	"github.com/mitchellh/cli"
	"github.com/stretchr/testify/require"
)

func TestIDPUpdateCommand_noTabs(t *testing.T) {
	t.Parallel()

	if strings.ContainsRune(New(cli.NewMockUi()).Help(), '\t') {
		t.Fatal("help has tabs")
	}
}

func TestIDPUpdateCommand(t *testing.T) {
	t.Parallel()

	testDir := testutil.TempDir(t, "acl")
	defer os.RemoveAll(testDir)

	a := agent.NewTestAgent(t, t.Name(), `
	primary_datacenter = "dc1"
	acl {
		enabled = true
		tokens {
			master = "root"
		}
	}`)

	a.Agent.LogWriter = logger.NewLogWriter(512)

	defer a.Shutdown()
	testrpc.WaitForLeader(t, a.RPC, "dc1")

	client := a.Client()

	ca := connect.TestCA(t, nil)
	ca2 := connect.TestCA(t, nil)

	t.Run("update without name", func(t *testing.T) {
		args := []string{
			"-http-addr=" + a.HTTPAddr(),
			"-token=root",
			"-kubernetes-host", "https://foo.internal:8443",
			"-kubernetes-ca-cert", ca.RootCert,
			"-kubernetes-service-account-jwt", goodJWT_A,
		}

		ui := cli.NewMockUi()
		cmd := New(ui)

		code := cmd.Run(args)
		require.Equal(t, code, 1)
		require.Contains(t, ui.ErrorWriter.String(), "Cannot update an identity provider without specifying the -name parameter")
	})

	t.Run("update nonexistent idp", func(t *testing.T) {
		args := []string{
			"-http-addr=" + a.HTTPAddr(),
			"-token=root",
			"-name=k8s",
			"-kubernetes-host", "https://foo.internal:8443",
			"-kubernetes-ca-cert", ca.RootCert,
			"-kubernetes-service-account-jwt", goodJWT_A,
		}

		ui := cli.NewMockUi()
		cmd := New(ui)

		code := cmd.Run(args)
		require.Equal(t, code, 1)
		require.Contains(t, ui.ErrorWriter.String(), "Identity Provider not found with name")
	})

	createIDP := func(t *testing.T) string {
		id, err := uuid.GenerateUUID()
		require.NoError(t, err)

		idpName := "k8s-" + id

		_, _, err = client.ACL().IdentityProviderCreate(
			&api.ACLIdentityProvider{
				Name:                        idpName,
				Type:                        "kubernetes",
				Description:                 "test idp",
				KubernetesHost:              "https://foo.internal:8443",
				KubernetesCACert:            ca.RootCert,
				KubernetesServiceAccountJWT: goodJWT_A,
			},
			&api.WriteOptions{Token: "root"},
		)
		require.NoError(t, err)

		return idpName
	}

	t.Run("update all fields", func(t *testing.T) {
		name := createIDP(t)

		args := []string{
			"-http-addr=" + a.HTTPAddr(),
			"-token=root",
			"-name=" + name,
			"-description", "updated description",
			"-kubernetes-host", "https://foo-new.internal:8443",
			"-kubernetes-ca-cert", ca2.RootCert,
			"-kubernetes-service-account-jwt", goodJWT_B,
		}

		ui := cli.NewMockUi()
		cmd := New(ui)

		code := cmd.Run(args)
		require.Equal(t, code, 0)
		require.Empty(t, ui.ErrorWriter.String())

		idp, _, err := client.ACL().IdentityProviderRead(
			name,
			&api.QueryOptions{Token: "root"},
		)
		require.NoError(t, err)
		require.NotNil(t, idp)
		require.Equal(t, "updated description", idp.Description)
		require.Equal(t, "https://foo-new.internal:8443", idp.KubernetesHost)
		require.Equal(t, ca2.RootCert, idp.KubernetesCACert)
		require.Equal(t, goodJWT_B, idp.KubernetesServiceAccountJWT)
	})

	ca2File := filepath.Join(testDir, "ca2.crt")
	require.NoError(t, ioutil.WriteFile(ca2File, []byte(ca2.RootCert), 0600))

	t.Run("update all fields with cert file", func(t *testing.T) {
		name := createIDP(t)

		args := []string{
			"-http-addr=" + a.HTTPAddr(),
			"-token=root",
			"-name=" + name,
			"-description", "updated description",
			"-kubernetes-host", "https://foo-new.internal:8443",
			"-kubernetes-ca-cert", "@" + ca2File,
			"-kubernetes-service-account-jwt", goodJWT_B,
		}

		ui := cli.NewMockUi()
		cmd := New(ui)

		code := cmd.Run(args)
		require.Equal(t, code, 0)
		require.Empty(t, ui.ErrorWriter.String())

		idp, _, err := client.ACL().IdentityProviderRead(
			name,
			&api.QueryOptions{Token: "root"},
		)
		require.NoError(t, err)
		require.NotNil(t, idp)
		require.Equal(t, "updated description", idp.Description)
		require.Equal(t, "https://foo-new.internal:8443", idp.KubernetesHost)
		require.Equal(t, ca2.RootCert, idp.KubernetesCACert)
		require.Equal(t, goodJWT_B, idp.KubernetesServiceAccountJWT)
	})

	t.Run("update all fields but k8s host", func(t *testing.T) {
		name := createIDP(t)

		args := []string{
			"-http-addr=" + a.HTTPAddr(),
			"-token=root",
			"-name=" + name,
			"-description", "updated description",
			"-kubernetes-ca-cert", ca2.RootCert,
			"-kubernetes-service-account-jwt", goodJWT_B,
		}

		ui := cli.NewMockUi()
		cmd := New(ui)

		code := cmd.Run(args)
		require.Equal(t, code, 0)
		require.Empty(t, ui.ErrorWriter.String())

		idp, _, err := client.ACL().IdentityProviderRead(
			name,
			&api.QueryOptions{Token: "root"},
		)
		require.NoError(t, err)
		require.NotNil(t, idp)
		require.Equal(t, "updated description", idp.Description)
		require.Equal(t, "https://foo.internal:8443", idp.KubernetesHost)
		require.Equal(t, ca2.RootCert, idp.KubernetesCACert)
		require.Equal(t, goodJWT_B, idp.KubernetesServiceAccountJWT)
	})

	t.Run("update all fields but k8s ca cert", func(t *testing.T) {
		name := createIDP(t)

		args := []string{
			"-http-addr=" + a.HTTPAddr(),
			"-token=root",
			"-name=" + name,
			"-description", "updated description",
			"-kubernetes-host", "https://foo-new.internal:8443",
			"-kubernetes-service-account-jwt", goodJWT_B,
		}

		ui := cli.NewMockUi()
		cmd := New(ui)

		code := cmd.Run(args)
		require.Equal(t, code, 0)
		require.Empty(t, ui.ErrorWriter.String())

		idp, _, err := client.ACL().IdentityProviderRead(
			name,
			&api.QueryOptions{Token: "root"},
		)
		require.NoError(t, err)
		require.NotNil(t, idp)
		require.Equal(t, "updated description", idp.Description)
		require.Equal(t, "https://foo-new.internal:8443", idp.KubernetesHost)
		require.Equal(t, ca.RootCert, idp.KubernetesCACert)
		require.Equal(t, goodJWT_B, idp.KubernetesServiceAccountJWT)
	})

	t.Run("update all fields but k8s jwt", func(t *testing.T) {
		name := createIDP(t)

		args := []string{
			"-http-addr=" + a.HTTPAddr(),
			"-token=root",
			"-name=" + name,
			"-description", "updated description",
			"-kubernetes-host", "https://foo-new.internal:8443",
			"-kubernetes-ca-cert", ca2.RootCert,
		}

		ui := cli.NewMockUi()
		cmd := New(ui)

		code := cmd.Run(args)
		require.Equal(t, code, 0)
		require.Empty(t, ui.ErrorWriter.String())

		idp, _, err := client.ACL().IdentityProviderRead(
			name,
			&api.QueryOptions{Token: "root"},
		)
		require.NoError(t, err)
		require.NotNil(t, idp)
		require.Equal(t, "updated description", idp.Description)
		require.Equal(t, "https://foo-new.internal:8443", idp.KubernetesHost)
		require.Equal(t, ca2.RootCert, idp.KubernetesCACert)
		require.Equal(t, goodJWT_A, idp.KubernetesServiceAccountJWT)
	})
}

func TestIDPUpdateCommand_noMerge(t *testing.T) {
	t.Parallel()

	testDir := testutil.TempDir(t, "acl")
	defer os.RemoveAll(testDir)

	a := agent.NewTestAgent(t, t.Name(), `
	primary_datacenter = "dc1"
	acl {
		enabled = true
		tokens {
			master = "root"
		}
	}`)

	a.Agent.LogWriter = logger.NewLogWriter(512)

	defer a.Shutdown()
	testrpc.WaitForLeader(t, a.RPC, "dc1")

	client := a.Client()

	ca := connect.TestCA(t, nil)
	ca2 := connect.TestCA(t, nil)

	t.Run("update without name", func(t *testing.T) {
		args := []string{
			"-http-addr=" + a.HTTPAddr(),
			"-token=root",
			"-no-merge",
			"-kubernetes-host", "https://foo.internal:8443",
			"-kubernetes-ca-cert", ca.RootCert,
			"-kubernetes-service-account-jwt", goodJWT_A,
		}

		ui := cli.NewMockUi()
		cmd := New(ui)

		code := cmd.Run(args)
		require.Equal(t, code, 1)
		require.Contains(t, ui.ErrorWriter.String(), "Cannot update an identity provider without specifying the -name parameter")
	})

	t.Run("update nonexistent idp", func(t *testing.T) {
		args := []string{
			"-http-addr=" + a.HTTPAddr(),
			"-token=root",
			"-no-merge",
			"-name=k8s",
			"-kubernetes-host", "https://foo.internal:8443",
			"-kubernetes-ca-cert", ca.RootCert,
			"-kubernetes-service-account-jwt", goodJWT_A,
		}

		ui := cli.NewMockUi()
		cmd := New(ui)

		code := cmd.Run(args)
		require.Equal(t, code, 1)
		require.Contains(t, ui.ErrorWriter.String(), "Identity Provider not found with name")
	})

	createIDP := func(t *testing.T) string {
		id, err := uuid.GenerateUUID()
		require.NoError(t, err)

		idpName := "k8s-" + id

		_, _, err = client.ACL().IdentityProviderCreate(
			&api.ACLIdentityProvider{
				Name:                        idpName,
				Type:                        "kubernetes",
				Description:                 "test idp",
				KubernetesHost:              "https://foo.internal:8443",
				KubernetesCACert:            ca.RootCert,
				KubernetesServiceAccountJWT: goodJWT_A,
			},
			&api.WriteOptions{Token: "root"},
		)
		require.NoError(t, err)

		return idpName
	}

	t.Run("update missing k8s host", func(t *testing.T) {
		name := createIDP(t)

		args := []string{
			"-http-addr=" + a.HTTPAddr(),
			"-token=root",
			"-no-merge",
			"-name=" + name,
			"-description", "updated description",
			"-kubernetes-ca-cert", ca2.RootCert,
			"-kubernetes-service-account-jwt", goodJWT_B,
		}

		ui := cli.NewMockUi()
		cmd := New(ui)

		code := cmd.Run(args)
		require.Equal(t, code, 1)
		require.Contains(t, ui.ErrorWriter.String(), "Missing required '-kubernetes-host' flag")
	})

	t.Run("update missing k8s ca cert", func(t *testing.T) {
		name := createIDP(t)

		args := []string{
			"-http-addr=" + a.HTTPAddr(),
			"-token=root",
			"-no-merge",
			"-name=" + name,
			"-description", "updated description",
			"-kubernetes-host", "https://foo-new.internal:8443",
			"-kubernetes-service-account-jwt", goodJWT_B,
		}

		ui := cli.NewMockUi()
		cmd := New(ui)

		code := cmd.Run(args)
		require.Equal(t, code, 1)
		require.Contains(t, ui.ErrorWriter.String(), "Missing required '-kubernetes-ca-cert' flag")
	})

	t.Run("update missing k8s jwt", func(t *testing.T) {
		name := createIDP(t)

		args := []string{
			"-http-addr=" + a.HTTPAddr(),
			"-token=root",
			"-no-merge",
			"-name=" + name,
			"-description", "updated description",
			"-kubernetes-host", "https://foo-new.internal:8443",
			"-kubernetes-ca-cert", ca2.RootCert,
		}

		ui := cli.NewMockUi()
		cmd := New(ui)

		code := cmd.Run(args)
		require.Equal(t, code, 1)
		require.Contains(t, ui.ErrorWriter.String(), "Missing required '-kubernetes-service-account-jwt' flag")
	})

	t.Run("update all fields", func(t *testing.T) {
		name := createIDP(t)

		args := []string{
			"-http-addr=" + a.HTTPAddr(),
			"-token=root",
			"-no-merge",
			"-name=" + name,
			"-description", "updated description",
			"-kubernetes-host", "https://foo-new.internal:8443",
			"-kubernetes-ca-cert", ca2.RootCert,
			"-kubernetes-service-account-jwt", goodJWT_B,
		}

		ui := cli.NewMockUi()
		cmd := New(ui)

		code := cmd.Run(args)
		require.Equal(t, code, 0)
		require.Empty(t, ui.ErrorWriter.String())

		idp, _, err := client.ACL().IdentityProviderRead(
			name,
			&api.QueryOptions{Token: "root"},
		)
		require.NoError(t, err)
		require.NotNil(t, idp)
		require.Equal(t, "updated description", idp.Description)
		require.Equal(t, "https://foo-new.internal:8443", idp.KubernetesHost)
		require.Equal(t, ca2.RootCert, idp.KubernetesCACert)
		require.Equal(t, goodJWT_B, idp.KubernetesServiceAccountJWT)
	})

	ca2File := filepath.Join(testDir, "ca2.crt")
	require.NoError(t, ioutil.WriteFile(ca2File, []byte(ca2.RootCert), 0600))

	t.Run("update all fields with cert file", func(t *testing.T) {
		name := createIDP(t)

		args := []string{
			"-http-addr=" + a.HTTPAddr(),
			"-token=root",
			"-no-merge",
			"-name=" + name,
			"-description", "updated description",
			"-kubernetes-host", "https://foo-new.internal:8443",
			"-kubernetes-ca-cert", "@" + ca2File,
			"-kubernetes-service-account-jwt", goodJWT_B,
		}

		ui := cli.NewMockUi()
		cmd := New(ui)

		code := cmd.Run(args)
		require.Equal(t, code, 0)
		require.Empty(t, ui.ErrorWriter.String())

		idp, _, err := client.ACL().IdentityProviderRead(
			name,
			&api.QueryOptions{Token: "root"},
		)
		require.NoError(t, err)
		require.NotNil(t, idp)
		require.Equal(t, "updated description", idp.Description)
		require.Equal(t, "https://foo-new.internal:8443", idp.KubernetesHost)
		require.Equal(t, ca2.RootCert, idp.KubernetesCACert)
		require.Equal(t, goodJWT_B, idp.KubernetesServiceAccountJWT)
	})
}

const goodJWT_A = "eyJhbGciOiJSUzI1NiIsImtpZCI6IiJ9.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJkZWZhdWx0Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZWNyZXQubmFtZSI6ImRlbW8tdG9rZW4ta21iOW4iLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC5uYW1lIjoiZGVtbyIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50LnVpZCI6Ijc2MDkxYWY0LTRiNTYtMTFlOS1hYzRiLTcwOGIxMTgwMWNiZSIsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDpkZWZhdWx0OmRlbW8ifQ.ZiAHjijBAOsKdum0Aix6lgtkLkGo9_Tu87dWQ5Zfwnn3r2FejEWDAnftTft1MqqnMzivZ9Wyyki5ZjQRmTAtnMPJuHC-iivqY4Wh4S6QWCJ1SivBv5tMZR79t5t8mE7R1-OHwst46spru1pps9wt9jsA04d3LpV0eeKYgdPTVaQKklxTm397kIMUugA6yINIBQ3Rh8eQqBgNwEmL4iqyYubzHLVkGkoP9MJikFI05vfRiHtYr-piXz6JFDzXMQj9rW6xtMmrBSn79ChbyvC5nz-Nj2rJPnHsb_0rDUbmXY5PpnMhBpdSH-CbZ4j8jsiib6DtaGJhVZeEQ1GjsFAZwQ"
const goodJWT_B = "eyJhbGciOiJSUzI1NiIsImtpZCI6IiJ9.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJkZWZhdWx0Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZWNyZXQubmFtZSI6ImNvbnN1bC1pZHAtdG9rZW4tcmV2aWV3LWFjY291bnQtdG9rZW4tbTYyZHMiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC5uYW1lIjoiY29uc3VsLWlkcC10b2tlbi1yZXZpZXctYWNjb3VudCIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50LnVpZCI6Ijc1ZTNjYmVhLTRiNTYtMTFlOS1hYzRiLTcwOGIxMTgwMWNiZSIsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDpkZWZhdWx0OmNvbnN1bC1pZHAtdG9rZW4tcmV2aWV3LWFjY291bnQifQ.uMb66tZ8d8gNzS8EnjlkzbrGKc5M-BESwS5B46IUbKfdMtajsCwgBXICytWKQ2X7wfm4QQykHVaElijBlO8QVvYeYzQE0uy75eH9EXNXmRh862YL_Qcy_doPC0R6FQXZW99S5Joc-3riKsq7N-sjEDBshOqyfDaGfan3hxaiV4Bv4hXXWRFUQ9aTAfPVvk1FQi21U9Fbml9ufk8kkk6gAmIEA_o7p-ve6WIhm48t7MJv314YhyVqXdrvmRykPdMwj4TfwSn3pTJ82P4NgSbXMJhwNkwIadJPZrM8EfN5ISpR4EW3jzP3IHtgQxrIovWQ9TQib1Z5zdRaLWaFVm6XaQ"
