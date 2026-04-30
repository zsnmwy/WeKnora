package handler

import "testing"

func TestMimeTypeByExtFreeMind(t *testing.T) {
	got := mimeTypeByExt("AI生成视频-Seedance 2.0使用指南.mm")
	want := "application/xml; charset=utf-8"
	if got != want {
		t.Fatalf("mimeTypeByExt(.mm) = %q, want %q", got, want)
	}
}
