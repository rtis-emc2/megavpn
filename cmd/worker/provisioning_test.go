package main

import "testing"

func TestSelectArtifactFilesForSpecificType(t *testing.T) {
	files := []generatedArtifactFile{
		{ArtifactType: "wg_conf", Filename: "client.conf"},
		{ArtifactType: "ovpn", Filename: "client.ovpn"},
	}

	selected := selectArtifactFiles(files, "wg_conf")
	if len(selected) != 1 || selected[0].ArtifactType != "wg_conf" {
		t.Fatalf("selected = %#v, want only wg_conf", selected)
	}
}

func TestSelectArtifactFilesForZipBundleReturnsBaseFilesForArchiveBuild(t *testing.T) {
	files := []generatedArtifactFile{
		{ArtifactType: "wg_conf", Filename: "client.conf"},
		{ArtifactType: "ovpn", Filename: "client.ovpn"},
	}

	selected := selectArtifactFiles(files, "zip_bundle")
	if len(selected) != len(files) {
		t.Fatalf("selected = %#v, want all base files for zip build", selected)
	}
}
