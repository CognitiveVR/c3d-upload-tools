package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/cognitivevr/c3d-upload-tools/internal/ego"
	"github.com/cognitivevr/c3d-upload-tools/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/internal/ui"
)

const (
	slotFlag   = "slot"
	egoDirFlag = "ego-dir"
	waitFlag   = "wait"
	yesFlag    = "yes"
)

var (
	egoWaitInterval = 5 * time.Second
	egoWaitTimeout  = 120 * time.Second
)

func newEgoCmd(rt *runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ego",
		Short: "Upload and manage egocentric (session replay) models",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newEgoUploadCmd(rt), newEgoListCmd(rt), newEgoDeleteCmd(rt))
	return cmd
}

func newEgoUploadCmd(rt *runtime) *cobra.Command {
	upload := &cobra.Command{
		Use:   "upload",
		Short: "Upload or replace one egocentric model slot",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEgoUpload(cmd, rt)
		},
	}
	flags := upload.Flags()
	flags.String(slotFlag, "", "slot to fill: head, controller_left, controller_right or a custom key (required)")
	flags.String(egoDirFlag, "", "directory holding the model, its .png, .jpg and .webp images and an optional ego.json (required)")
	flags.String(sceneIDFlag, "", "the scene to upload to (or set C3D_SCENE_ID)")
	flags.Int(versionFlag, 0, "scene version number; the latest version when omitted")
	flags.Bool(waitFlag, false, "wait up to 120 seconds for the gateway to mark the model optimized")
	flags.Bool(dryRunFlag, false, "print the request without sending it")
	return upload
}

type egoUploadResult struct {
	SceneID       string `json:"sceneId"`
	VersionNumber int    `json:"versionNumber"`
	Slot          string `json:"slot"`
	Model         string `json:"model"`
	FileCount     int    `json:"fileCount"`
	Optimized     *bool  `json:"optimized"`
}

func runEgoUpload(cmd *cobra.Command, rt *runtime) error {
	flags := cmd.Flags()
	slot, _ := flags.GetString(slotFlag)
	dir, _ := flags.GetString(egoDirFlag)
	wait, _ := flags.GetBool(waitFlag)
	dryRun, _ := flags.GetBool(dryRunFlag)
	switch {
	case slot == "":
		return usageErrorf("--%s is required", slotFlag)
	case dir == "":
		return usageErrorf("--%s is required", egoDirFlag)
	}
	wanted, versionGiven, err := versionFlagValue(flags)
	if err != nil {
		return err
	}
	if err := ego.ValidateSlot(slot); err != nil {
		return &usageError{message: err.Error()}
	}
	sceneID, err := requireSceneID(rt)
	if err != nil {
		return err
	}

	plan, err := ego.Plan(dir, slot)
	if err != nil {
		return planFailure(rt, err)
	}
	for _, warning := range plan.Warnings {
		rt.log.Warn("%s", warning)
	}

	if dryRun {
		version := versionText(wanted, versionGiven)
		if wait {
			rt.log.Info("--%s would then poll %s?version=%s for up to %s seconds", waitFlag, ego.ListPath(sceneID), version, seconds(egoWaitTimeout))
		}
		return printEgoDryRun(rt, plan, sceneID, version)
	}

	ctx := cmd.Context()
	if err := preflight(ctx, rt); err != nil {
		return err
	}
	versionNumber := wanted
	if !versionGiven {
		if versionNumber, err = latestVersionNumber(ctx, rt, sceneID); err != nil {
			return err
		}
	}

	client := rt.client()
	uploaded, err := ego.Upload(ctx, client, plan, sceneID, versionNumber, rt.log, uploadProgress(rt, plan.TotalBytes))
	if err != nil {
		return err
	}
	result := egoUploadResult{
		SceneID: sceneID, VersionNumber: versionNumber, Slot: uploaded.Slot, Model: uploaded.Model, FileCount: uploaded.FileCount,
	}
	if wait {
		rt.log.Info("Waiting up to %s seconds for slot %s to be optimized", seconds(egoWaitTimeout), slot)
		optimized, err := ego.WaitOptimized(ctx, client, sceneID, slot, versionNumber, egoWaitInterval, egoWaitTimeout, rt.log)
		if err != nil {
			if printErr := printEgoUploadResult(rt, result); printErr != nil {
				return printErr
			}
			if cause := ctx.Err(); cause != nil {
				return &codedError{code: exitFailure, kind: gateway.Interrupted.String(), message: fmt.Sprintf(
					"The model was uploaded; waiting for optimization was cut off (%v). Check with: %s ego list --scene-id %s",
					cause, binaryName, sceneID)}
			}
			return err
		}
		result.Optimized = &optimized
		if !optimized {
			rt.log.Warn("%s", optimizationPendingMessage(egoWaitTimeout))
		}
	}
	return printEgoUploadResult(rt, result)
}

func seconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', -1, 64)
}

func optimizationPendingMessage(timeout time.Duration) string {
	return "Optimization still pending after " + seconds(timeout) + " seconds; small models are never marked optimized."
}

func printEgoUploadResult(rt *runtime, result egoUploadResult) error {
	out := rt.streams.Stdout
	if rt.settings.Format == formatJSON {
		return json.NewEncoder(out).Encode(result)
	}
	text := fmt.Sprintf("Slot: %s\nModel: %s\nFiles: %d\n", result.Slot, result.Model, result.FileCount)
	if result.Optimized != nil && *result.Optimized {
		text += "Optimized: true\n"
	}
	_, err := io.WriteString(out, text)
	return err
}

type egoDryRun struct {
	dryRunRequest
	Slot string `json:"slot"`
}

func printEgoDryRun(rt *runtime, plan *ego.UploadPlan, sceneID, version string) error {
	request := egoDryRun{
		dryRunRequest: newDryRunRequest(rt, http.MethodPost, ego.SlotPath(sceneID, plan.Slot)+"?version="+version, plan.Parts),
		Slot:          plan.Slot,
	}
	return printDryRun(rt, request.dryRunRequest, request, "")
}

func newEgoListCmd(rt *runtime) *cobra.Command {
	list := &cobra.Command{
		Use:   "list",
		Short: "Show the ego slots of a scene version",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEgoList(cmd, rt)
		},
	}
	flags := list.Flags()
	flags.String(sceneIDFlag, "", "the scene (or set C3D_SCENE_ID)")
	flags.Int(versionFlag, 0, "version number; the latest version when omitted")
	return list
}

func runEgoList(cmd *cobra.Command, rt *runtime) error {
	wanted, versionGiven, err := versionFlagValue(cmd.Flags())
	if err != nil {
		return err
	}
	sceneID, err := requireSceneID(rt)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	if err := preflight(ctx, rt); err != nil {
		return err
	}
	chosen, err := chooseVersion(ctx, rt, sceneID, wanted, versionGiven)
	if err != nil {
		return err
	}
	listing, err := ego.List(ctx, rt.client(), sceneID, chosen.Number)
	if err != nil {
		return err
	}
	if rt.settings.Format == formatJSON {
		return json.NewEncoder(rt.streams.Stdout).Encode(listing)
	}
	return printEgoTable(rt.streams.Stdout, listing)
}

// printEgoTable writes one row per known slot, "empty" when it holds no
// model, then any other slot the gateway returned, sorted by key.
func printEgoTable(out io.Writer, listing ego.Listing) error {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "SLOT\tMODEL\tFILES\tMANIFEST\tOPTIMIZED")
	for _, key := range ego.KnownSlots {
		if s, ok := listing.Find(key); ok {
			writeEgoRow(w, s)
		} else {
			_, _ = fmt.Fprintf(w, "%s\tempty\t\t\t\n", key)
		}
	}
	var extra []ego.Slot
	for _, s := range listing.Egos {
		if !slices.Contains(ego.KnownSlots, s.Key) {
			extra = append(extra, s)
		}
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i].Key < extra[j].Key })
	for _, s := range extra {
		writeEgoRow(w, s)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	lines := strings.SplitAfter(b.String(), "\n")
	for i, line := range lines {
		if trimmed, ok := strings.CutSuffix(line, "\n"); ok {
			lines[i] = strings.TrimRight(trimmed, " ") + "\n"
		}
	}
	_, err := io.WriteString(out, strings.Join(lines, ""))
	return err
}

func writeEgoRow(w io.Writer, s ego.Slot) {
	_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%t\t%t\n", ui.Printable(s.Key), ui.Printable(s.Model), len(s.Files), s.Manifest, s.Optimized)
}

func newEgoDeleteCmd(rt *runtime) *cobra.Command {
	del := &cobra.Command{
		Use:   "delete",
		Short: "Remove a slot, reverting session replay to the bundled model",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEgoDelete(cmd, rt)
		},
	}
	flags := del.Flags()
	flags.String(slotFlag, "", "slot to empty (required)")
	flags.String(sceneIDFlag, "", "the scene (or set C3D_SCENE_ID)")
	flags.Int(versionFlag, 0, "version number; the latest version when omitted")
	flags.Bool(yesFlag, false, "delete without asking for confirmation")
	return del
}

type egoDeleteResult struct {
	SceneID       string `json:"sceneId"`
	VersionNumber int    `json:"versionNumber"`
	Slot          string `json:"slot"`
	Deleted       bool   `json:"deleted"`
	Existed       bool   `json:"existed"`
}

func runEgoDelete(cmd *cobra.Command, rt *runtime) error {
	flags := cmd.Flags()
	slot, _ := flags.GetString(slotFlag)
	yes, _ := flags.GetBool(yesFlag)
	if slot == "" {
		return usageErrorf("--%s is required", slotFlag)
	}
	wanted, versionGiven, err := versionFlagValue(flags)
	if err != nil {
		return err
	}
	if err := ego.ValidateSlot(slot); err != nil {
		return &usageError{message: err.Error()}
	}
	sceneID, err := requireSceneID(rt)
	if err != nil {
		return err
	}
	if !yes && !rt.streams.Interactive {
		return usageErrorf("refusing to delete without --%s when not running interactively", yesFlag)
	}

	ctx := cmd.Context()
	if err := preflight(ctx, rt); err != nil {
		return err
	}
	chosen, err := chooseVersion(ctx, rt, sceneID, wanted, versionGiven)
	if err != nil {
		return err
	}
	if !yes {
		ok, err := confirm(ctx, rt.streams, fmt.Sprintf("Delete ego slot %q from scene %s version %d? [y/N] ", slot, sceneID, chosen.Number))
		if err != nil {
			return err
		}
		if !ok {
			_, err := io.WriteString(rt.streams.Stderr, "Aborted.\n")
			return err
		}
	}

	deleted, err := ego.Delete(ctx, rt.client(), sceneID, slot, chosen.Number)
	if err != nil {
		return err
	}
	if rt.settings.Format == formatJSON {
		return json.NewEncoder(rt.streams.Stdout).Encode(egoDeleteResult{
			SceneID: sceneID, VersionNumber: chosen.Number, Slot: slot, Deleted: true, Existed: deleted.Existed,
		})
	}
	if deleted.Existed {
		rt.log.Info("Deleted slot %s from version %d", slot, chosen.Number)
	} else {
		rt.log.Info("Slot %s was already empty in version %d", slot, chosen.Number)
	}
	return nil
}

// confirm writes prompt on stderr and reports whether the next line of stdin
// is y or yes, in any case. Once ctx is done it returns ctx's error without
// waiting for the line.
func confirm(ctx context.Context, streams IO, prompt string) (bool, error) {
	if _, err := io.WriteString(streams.Stderr, prompt); err != nil || streams.Stdin == nil {
		return false, nil
	}
	lines := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(streams.Stdin).ReadString('\n')
		lines <- line
	}()
	select {
	case <-ctx.Done():
		_, _ = io.WriteString(streams.Stderr, "\n")
		return false, ctx.Err()
	case line := <-lines:
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		}
		return false, nil
	}
}
