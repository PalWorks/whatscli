// SPDX-License-Identifier: AGPL-3.0-only

package main

import "runtime/debug"

// appVersion is the version shown in the UI: the build-time VERSION, else the
// module version recorded by go install (for example v1.6.1), else "dev".
func appVersion() string {
	info, ok := debug.ReadBuildInfo()
	return versionFrom(VERSION, info, ok)
}

func versionFrom(injected string, info *debug.BuildInfo, ok bool) string {
	if injected != "" {
		return injected
	}
	if ok && info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
