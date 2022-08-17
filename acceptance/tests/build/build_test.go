//go:build acceptance
// +build acceptance

package build_test

import (
	"fmt"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/buildpacks/pack/acceptance/assertions"
	"github.com/buildpacks/pack/acceptance/harness"
	h "github.com/buildpacks/pack/testhelpers"
)

func test_runnable_rebuildable_inspectable(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	registry := th.Registry()
	imageManager := th.ImageManager()
	runImageName := th.RunImageName()
	runImageMirror := th.RunImageMirror()

	assert := h.NewAssertionManager(t)
	assertImage := assertions.NewImageAssertionManager(t, imageManager, &registry)

	pack := combo.Pack()

	appPath := filepath.Join("..", "..", "testdata", "mock_app")

	repo := "some-org/" + h.RandString(10)
	repoName := registry.RepoName(repo)

	output := pack.RunSuccessfully(
		"build", repoName,
		"-p", appPath,
	)

	assertOutput := assertions.NewOutputAssertionManager(t, output)

	assertOutput.ReportsSuccessfulImageBuild(repoName)
	assertOutput.ReportsUsingBuildCacheVolume()
	assertOutput.ReportsSelectingRunImageMirror(runImageMirror)

	t.Log("app is runnable")
	assertImage.RunsWithOutput(repoName, "Launch Dep Contents", "Cached Dep Contents")

	t.Log("it uses the run image as a base image")
	assertImage.HasBaseImage(repoName, runImageName)

	t.Log("sets the run image metadata")
	assertImage.HasLabelWithData(repoName, "io.buildpacks.lifecycle.metadata", fmt.Sprintf(`"stack":{"runImage":{"image":"%s","mirrors":["%s"]}}}`, runImageName, runImageMirror))

	t.Log("sets the source metadata")
	assertImage.HasLabelWithData(repoName, "io.buildpacks.project.metadata", (`{"source":{"type":"project","version":{"declared":"1.0.2"},"metadata":{"url":"https://github.com/buildpacks/pack"}}}`))

	t.Log("registry is empty")
	assertImage.NotExistsInRegistry(repo)

	t.Log("add a local mirror")
	localRunImageMirror := registry.RepoName("pack-test/run-mirror")
	imageManager.TagImage(runImageName, localRunImageMirror)
	defer imageManager.CleanupImages(localRunImageMirror)

	pack.JustRunSuccessfully("config", "run-image-mirrors", "add", runImageName, "-m", localRunImageMirror)
	defer pack.JustRunSuccessfully("config", "run-image-mirrors", "remove", runImageName)

	t.Log("rebuild")
	output = pack.RunSuccessfully(
		"build", repoName,
		"-p", appPath,
	)
	assertOutput = assertions.NewOutputAssertionManager(t, output)
	assertOutput.ReportsSuccessfulImageBuild(repoName)
	assertOutput.ReportsSelectingRunImageMirrorFromLocalConfig(localRunImageMirror)
	cachedLaunchLayer := "simple/layers:cached-launch-layer"

	assertLifecycleOutput := assertions.NewLifecycleOutputAssertionManager(t, output)
	assertLifecycleOutput.ReportsRestoresCachedLayer(cachedLaunchLayer)
	assertLifecycleOutput.ReportsExporterReusingUnchangedLayer(cachedLaunchLayer)
	assertLifecycleOutput.ReportsCacheReuse(cachedLaunchLayer)

	t.Log("app is runnable")
	assertImage.RunsWithOutput(repoName, "Launch Dep Contents", "Cached Dep Contents")

	t.Log("rebuild with --clear-cache")
	output = pack.RunSuccessfully("build",
		repoName,
		"-p", appPath,
		"--clear-cache",
	)

	assertOutput = assertions.NewOutputAssertionManager(t, output)
	assertOutput.ReportsSuccessfulImageBuild(repoName)
	assertLifecycleOutput = assertions.NewLifecycleOutputAssertionManager(t, output)
	assertLifecycleOutput.ReportsExporterReusingUnchangedLayer(cachedLaunchLayer)
	assertLifecycleOutput.ReportsCacheCreation(cachedLaunchLayer)

	t.Log("cacher adds layers")
	assert.Matches(output, regexp.MustCompile(`(?i)Adding cache layer 'simple/layers:cached-launch-layer'`))

	t.Log("inspecting image")
	inspectCmd := "inspect"
	if !pack.Supports("inspect") {
		inspectCmd = "inspect-image"
	}

	var (
		webCommand      string
		helloCommand    string
		helloArgs       []string
		helloArgsPrefix string
		imageWorkdir    string
	)
	if imageManager.HostOS() == "windows" {
		webCommand = ".\\run"
		helloCommand = "cmd"
		helloArgs = []string{"/c", "echo hello world"}
		helloArgsPrefix = " "
		imageWorkdir = "c:\\workspace"
	} else {
		webCommand = "./run"
		helloCommand = "echo"
		helloArgs = []string{"hello", "world"}
		helloArgsPrefix = ""
		imageWorkdir = "/workspace"
	}

	formats := []compareFormat{
		{
			extension:   "json",
			compareFunc: assert.EqualJSON,
			outputArg:   "json",
		},
		{
			extension:   "yaml",
			compareFunc: assert.EqualYAML,
			outputArg:   "yaml",
		},
		{
			extension:   "toml",
			compareFunc: assert.EqualTOML,
			outputArg:   "toml",
		},
	}
	for _, format := range formats {
		t.Logf("inspecting image %s format", format.outputArg)

		output = pack.RunSuccessfully(inspectCmd, repoName, "--output", format.outputArg)
		expectedOutput := pack.FixtureManager().TemplateFixture(
			fmt.Sprintf("inspect_image_local_output.%s", format.extension),
			map[string]interface{}{
				"image_name":             repoName,
				"base_image_id":          h.ImageID(t, runImageMirror),
				"base_image_top_layer":   h.TopLayerDiffID(t, runImageMirror),
				"run_image_local_mirror": localRunImageMirror,
				"run_image_mirror":       runImageMirror,
				"web_command":            webCommand,
				"hello_command":          helloCommand,
				"hello_args":             helloArgs,
				"hello_args_prefix":      helloArgsPrefix,
				"image_workdir":          imageWorkdir,
			},
		)

		format.compareFunc(output, expectedOutput)
	}
}

func test_app_zipfile(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	registry := th.Registry()

	pack := combo.Pack()

	repoName := registry.RepoName("sample/" + h.RandString(10))
	appPath := filepath.Join("..", "..", "testdata", "mock_app.zip")
	output := pack.RunSuccessfully("build", repoName, "-p", appPath)
	assertions.NewOutputAssertionManager(t, output).ReportsSuccessfulImageBuild(repoName)
}

type compareFormat struct {
	extension   string
	compareFunc func(string, string)
	outputArg   string
}
