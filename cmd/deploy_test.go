package cmd

import (
	"os"
	"strings"
	"testing"

	gothic_cli "github.com/felipegenef/gothicframework/v2/pkg/cli"
	"github.com/spf13/cobra"
)

// writeAppID seeds the app-id file GetAppId reads.
func writeAppID(t *testing.T, id string) {
	t.Helper()
	if err := os.MkdirAll(".gothicCli", 0o755); err != nil {
		t.Fatalf("mkdir .gothicCli: %v", err)
	}
	if err := os.WriteFile(".gothicCli/app-id.txt", []byte(id), 0o644); err != nil {
		t.Fatalf("write app-id: %v", err)
	}
}

func newTestDeployCommand() DeployCommand {
	cli := gothic_cli.NewCli()
	return newDeployCommandCli(&cli)
}

func TestNewDeployCommandCli(t *testing.T) {
	cmd := newTestDeployCommand()
	if cmd.cli == nil {
		t.Fatal("expected cli set")
	}
	if len(cmd.allowedActions) != 2 {
		t.Errorf("expected 2 allowed actions, got %v", cmd.allowedActions)
	}
}

func TestIsValidAction(t *testing.T) {
	cmd := newTestDeployCommand()
	cases := map[string]bool{
		"deploy":  true,
		"delete":  true,
		"destroy": false,
		"":        false,
		"DEPLOY":  false,
	}
	for action, want := range cases {
		if got := cmd.isValidAction(action); got != want {
			t.Errorf("isValidAction(%q) = %v, want %v", action, got, want)
		}
	}
}

func TestDeployCleanupRemovesFiles(t *testing.T) {
	chdirTemp(t)
	for _, f := range []string{"Dockerfile", "template.yaml", "samconfig.toml"} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	cmd := newTestDeployCommand()
	cmd.cleanup()
	for _, f := range []string{"Dockerfile", "template.yaml", "samconfig.toml"} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("expected %s removed", f)
		}
	}
}

func TestDeployCleanupTolerantOfMissingFiles(t *testing.T) {
	chdirTemp(t)
	// No files present: cleanup must not panic and must print errors gracefully.
	cmd := newTestDeployCommand()
	cmd.cleanup()
}

func TestDeployRunEInvalidAction(t *testing.T) {
	chdirTemp(t)
	runE := newDeployCommand(gothic_cli.NewCli())
	c := &cobra.Command{}
	c.Flags().StringP("stage", "s", "dev", "")
	c.Flags().StringP("action", "a", "destroy", "")
	if err := runE(c, nil); err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestDeployRunEInvalidStage(t *testing.T) {
	chdirTemp(t)
	runE := newDeployCommand(gothic_cli.NewCli())
	c := &cobra.Command{}
	c.Flags().StringP("stage", "s", "bad stage!", "")
	c.Flags().StringP("action", "a", "deploy", "")
	if err := runE(c, nil); err == nil {
		t.Fatal("expected error for invalid stage name")
	}
}

func TestDeployInvalidStageName(t *testing.T) {
	chdirTemp(t)
	cmd := newTestDeployCommand()
	if err := cmd.Deploy("bad stage!", "deploy"); err == nil {
		t.Fatal("expected error for invalid stage name")
	}
}

func TestDeploySetupSucceeds(t *testing.T) {
	chdirTemp(t)
	writeAppID(t, "abc123")
	writeConfig(t, `{
		"projectName":"demo",
		"goModuleName":"demo",
		"deploy":{
			"serverMemory":512,
			"serverTimeout":30,
			"region":"us-east-1",
			"profile":"default",
			"customDomain":false,
			"stages":{"dev":{"env":{"FOO":"bar","NUM":42}}}
		}
	}`)

	cmd := newTestDeployCommand()
	if err := cmd.setup("dev"); err != nil {
		t.Fatalf("setup() error: %v", err)
	}
}

func TestDeploySetupFailsWithoutDeployConfig(t *testing.T) {
	chdirTemp(t)
	writeAppID(t, "abc123")
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo"}`)

	cmd := newTestDeployCommand()
	if err := cmd.setup("dev"); err == nil {
		t.Fatal("expected error when deploy config missing")
	}
}

func TestDeploySetupFailsWithoutAppID(t *testing.T) {
	chdirTemp(t)
	// Deploy config present but no .gothicCli/app-id.txt: GetAppId fails.
	writeConfig(t, `{
		"projectName":"demo","goModuleName":"demo",
		"deploy":{"region":"us-east-1","profile":"default","stages":{"dev":{}}}
	}`)

	cmd := newTestDeployCommand()
	if err := cmd.setup("dev"); err == nil {
		t.Fatal("expected error when app-id missing")
	}
}

func TestDeploySetupCustomDomainRequiresFields(t *testing.T) {
	chdirTemp(t)
	writeAppID(t, "abc123")
	// customDomain true but no customDomain/hostedZoneId in stage env -> error.
	writeConfig(t, `{
		"projectName":"demo","goModuleName":"demo",
		"deploy":{"region":"us-east-1","profile":"default","customDomain":true,"stages":{"dev":{}}}
	}`)

	cmd := newTestDeployCommand()
	if err := cmd.setup("dev"); err == nil {
		t.Fatal("expected error when custom domain fields missing")
	}
}

func TestDeployProceedsUntilWasmScan(t *testing.T) {
	bin := writeFakeTailwind(t, true)
	chdirTemp(t)
	scaffoldSrc(t)
	writeAppID(t, "abc123")
	writeConfig(t, `{
		"projectName":"demo","goModuleName":"demo","tailwindBinary":"`+bin+`",
		"deploy":{"region":"us-east-1","profile":"default","stages":{"dev":{}}}
	}`)

	// setup + Templ.Render + Router.Render + Tailwind.Build (fake) all succeed,
	// then Wasm.ScanPages fails outside a real Go module. This exercises the
	// bulk of Deploy() without ever reaching AWS/SAM. cleanup() also runs via
	// the deferred call.
	cmd := newTestDeployCommand()
	err := cmd.Deploy("dev", "deploy")
	if err == nil {
		t.Fatal("expected Deploy to fail at wasm scan stage")
	}
	// Assert the failure originates from the wasm-scan stage specifically, not
	// from some earlier step. Deploy() wraps the scan error with "wasm:".
	if !strings.Contains(err.Error(), "wasm") {
		t.Fatalf("expected wasm-scan error, got: %v", err)
	}
}

// TestDeploySetupFailsWithBadStageBucketName exercises the bucket-name
// validation path that Deploy() runs at line `originBucketName := ...`.
//
// The bucket name is built as projectName + "-" + stage + "-" + appID. An
// uppercase project name produces an invalid S3 bucket name. In Deploy() this
// validation sits after the wasm/SAM build stages, which require real external
// tooling and so cannot be reached in a unit test. We therefore drive the same
// ValidateBucketName call with an identically-constructed name and assert the
// error is bucket-name-specific.
func TestDeploySetupFailsWithBadStageBucketName(t *testing.T) {
	const projectName = "Demo" // uppercase -> invalid S3 bucket name
	const stage = "dev"
	const appID = "abc123"

	originBucketName := projectName + "-" + stage + "-" + appID
	err := gothic_cli.ValidateBucketName(originBucketName)
	if err == nil {
		t.Fatalf("expected bucket-name validation to fail for %q", originBucketName)
	}
	if !strings.Contains(err.Error(), "bucket name") {
		t.Fatalf("expected a bucket-name-specific error, got: %v", err)
	}
}

func TestDeploySetupCustomDomainNonUsEast1RequiresArn(t *testing.T) {
	chdirTemp(t)
	writeAppID(t, "abc123")
	// customDomain true, region != us-east-1, no certificateArn -> error.
	writeConfig(t, `{
		"projectName":"demo","goModuleName":"demo",
		"deploy":{"region":"eu-west-1","profile":"default","customDomain":true,"stages":{"dev":{"customDomain":"example.com","hostedZoneId":"Z123"}}}
	}`)

	cmd := newTestDeployCommand()
	if err := cmd.setup("dev"); err == nil {
		t.Fatal("expected error when non us-east-1 custom domain lacks certificateArn")
	}
}
