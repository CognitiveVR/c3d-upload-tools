package cli

import (
	"context"
	"fmt"
	"os"

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
	if _, err := rt.streams.Stdout.Write(pretty); err != nil {
		return err
	}

	if writeList {
		file := sceneID + "_object_list.json"
		if err := os.WriteFile(file, pretty, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", file, err)
		}
		rt.log.Info("Wrote the object list to %s", file)
	}
	if writeManifest {
		m, err := manifest.FromObjectList(raw)
		if err != nil {
			return err
		}
		file := manifest.DefaultPath(sceneID)
		if err := m.Save(file); err != nil {
			return err
		}
		rt.log.Info("Wrote %d objects to %s", len(m.Objects), file)
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
