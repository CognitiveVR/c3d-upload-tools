package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cognitivevr/c3d-upload-tools/internal/manifest"
	"github.com/cognitivevr/c3d-upload-tools/internal/object"
	"github.com/cognitivevr/c3d-upload-tools/internal/scene"
)

const (
	writeManifestFlag = "write-manifest"
	writeListFlag     = "write-list"
)

func newObjectListCmd(rt *runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the objects of a scene version",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runObjectList(cmd, rt)
		},
	}
	flags := cmd.Flags()
	flags.String(sceneIDFlag, "", "the scene (or set C3D_SCENE_ID)")
	flags.Int(versionFlag, 0, "version number; the latest version when omitted")
	flags.Bool(writeManifestFlag, false, "also write the list as <scene id>_object_manifest.json, replacing that file")
	flags.Bool(writeListFlag, false, "also write the raw list as <scene id>_object_list.json")
	return cmd
}

func runObjectList(cmd *cobra.Command, rt *runtime) error {
	flags := cmd.Flags()
	wanted, _ := flags.GetInt(versionFlag)
	writeManifest, _ := flags.GetBool(writeManifestFlag)
	writeList, _ := flags.GetBool(writeListFlag)
	if flags.Changed(versionFlag) && wanted < 1 {
		return usageErrorf("invalid --%s %d: want a version number of 1 or more", versionFlag, wanted)
	}
	sceneID, err := requireSceneID(rt)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if err := preflight(ctx, rt); err != nil {
		return err
	}
	chosen, err := chooseVersion(ctx, rt, sceneID, wanted, flags.Changed(versionFlag))
	if err != nil {
		return err
	}
	raw, err := object.List(ctx, rt.client(), chosen.ID)
	if err != nil {
		return err
	}
	pretty, err := object.Indent(raw)
	if err != nil {
		return err
	}
	var reshaped *manifest.Manifest
	if writeManifest {
		if reshaped, err = manifest.FromObjectList(raw); err != nil {
			return err
		}
	}
	if _, err := rt.streams.Stdout.Write(pretty); err != nil {
		return err
	}

	if writeList {
		file := sceneID + "_object_list.json"
		if err := writeFileAtomic(file, pretty); err != nil {
			return err
		}
		rt.log.Info("Wrote the object list to %s", file)
	}
	if writeManifest {
		file := manifest.DefaultPath(sceneID)
		if err := reshaped.Save(file); err != nil {
			return err
		}
		rt.log.Info("Wrote %d objects to %s", len(reshaped.Objects), file)
	}
	return nil
}

// writeFileAtomic writes data through a temporary file in the same directory
// and a rename, and never leaves the temporary file behind.
func writeFileAtomic(file string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(file), "."+filepath.Base(file)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("write %s: %w", file, err)
	}
	tmpName := tmp.Name()
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmpName, 0o644)
	}
	if err == nil {
		err = os.Rename(tmpName, file)
	}
	if err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("write %s: %w", file, err)
	}
	return nil
}

// chooseVersion returns the version numbered wanted when given, else the
// scene's latest one.
func chooseVersion(ctx context.Context, rt *runtime, sceneID string, wanted int, given bool) (scene.Version, error) {
	info, err := scene.Describe(ctx, rt.client(), sceneID)
	if err != nil {
		return scene.Version{}, err
	}
	return pickVersion(info, sceneID, wanted, given)
}

func pickVersion(info scene.Info, sceneID string, wanted int, given bool) (scene.Version, error) {
	if given {
		v, ok := scene.VersionByNumber(info, wanted)
		if !ok {
			return v, &codedError{code: exitNotFound, kind: "version_not_found", message: unknownVersionMessage(info, sceneID, wanted)}
		}
		return v, nil
	}
	v, ok := scene.LatestVersion(info)
	if !ok {
		return v, &codedError{code: exitNotFound, kind: "no_versions", message: fmt.Sprintf("scene %s has no versions", sceneID)}
	}
	return v, nil
}
