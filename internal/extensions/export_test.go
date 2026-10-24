package extensions

// InstallFrom installs without the extra check. Only tests use it: real
// installs go through InstallChecked with the agent recipe check.
func InstallFrom(src, extDir string) (Manifest, error) {
	return InstallChecked(src, extDir, nil)
}
