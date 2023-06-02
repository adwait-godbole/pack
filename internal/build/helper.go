package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"

	"github.com/buildpacks/pack/pkg/logging"
)

const (
	DockerfileKindBuild = "build"
	DockerfileKindRun   = "run"
)

type Extensions struct {
	Extensions []buildpack.GroupElement
}

func (extensions *Extensions) DockerFiles(kind string, path string, logger logging.Logger) ([]buildpack.DockerfileInfo, error) {
	var dockerfiles []buildpack.DockerfileInfo
	for _, ext := range extensions.Extensions {
		dockerfile, err := extensions.ReadDockerFile(path, kind, ext.ID)
		if err != nil {
			return nil, err
		}
		if dockerfile != nil {
			logger.Debugf("Found %s Dockerfile for extension '%s'", kind, ext.ID)
			switch kind {
			case DockerfileKindBuild:
				// will implement later
			case DockerfileKindRun:
				buildpack.ValidateRunDockerfile(dockerfile, logger)
			default:
				return nil, fmt.Errorf("unknown dockerfile kind: %s", kind)
			}
			dockerfiles = append(dockerfiles, *dockerfile)
		}
	}
	return dockerfiles, nil
}

func (extensions *Extensions) ReadDockerFile(path string, kind string, extID string) (*buildpack.DockerfileInfo, error) {
	dockerfilePath := filepath.Join(path, kind, escapeID(extID), "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err != nil {
		return nil, nil
	}
	return &buildpack.DockerfileInfo{
		ExtensionID: extID,
		Kind:        kind,
		Path:        dockerfilePath,
	}, nil
}

func (extensions *Extensions) SetExtensions(path string, logger logging.Logger) error {
	groupExt, err := readExtensionsGroup(path)
	if err != nil {
		return fmt.Errorf("reading group: %w", err)
	}
	for i := range groupExt {
		groupExt[i].Extension = true
	}
	for _, groupEl := range groupExt {
		if err = cmd.VerifyBuildpackAPI(groupEl.Kind(), groupEl.String(), groupEl.API, logger); err != nil {
			return err
		}
	}
	extensions.Extensions = groupExt
	fmt.Println("extensions.Extensions", extensions.Extensions)
	return nil
}

func readExtensionsGroup(path string) ([]buildpack.GroupElement, error) {
	var group buildpack.Group
	_, err := toml.DecodeFile(filepath.Join(path, "group.toml"), &group)
	for e := range group.GroupExtensions {
		group.GroupExtensions[e].Extension = true
		group.GroupExtensions[e].Optional = true
	}
	return group.GroupExtensions, err
}

func escapeID(id string) string {
	return strings.ReplaceAll(id, "/", "_")
}
