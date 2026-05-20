package session

import (
	"context"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeDocumentReader struct {
	result *types.ReadResult
	err    error
}

func (f *fakeDocumentReader) Read(context.Context, *types.ReadRequest) (*types.ReadResult, error) {
	return f.result, f.err
}

func (f *fakeDocumentReader) Reconnect(string) error {
	return nil
}

func (f *fakeDocumentReader) IsConnected() bool {
	return true
}

func (f *fakeDocumentReader) ListEngines(context.Context, map[string]string) ([]types.ParserEngineInfo, error) {
	return nil, nil
}

func TestProcessWithDocumentReaderReturnsRemoteError(t *testing.T) {
	processor := &AttachmentProcessor{
		documentReader: &fakeDocumentReader{
			result: &types.ReadResult{Error: "Unsupported file type: .pdf"},
		},
	}
	attachment := &types.MessageAttachment{}

	err := processor.processWithDocumentReader(context.Background(), []byte("pdf"), "test.pdf", ".pdf", attachment, 1)

	if err == nil {
		t.Fatal("processWithDocumentReader() error = nil, want remote error")
	}
	if !strings.Contains(err.Error(), "Unsupported file type: .pdf") {
		t.Fatalf("processWithDocumentReader() error = %q, want remote error details", err.Error())
	}
}
