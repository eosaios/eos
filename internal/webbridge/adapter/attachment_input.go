package adapter

import (
	"github.com/eosaios/eos/pkg/attachments"
	"github.com/eosaios/eos/pkg/coreapi"
	"strings"
)

// CoreAPITurnInput keeps documents as tool-readable paths and encodes images.
func CoreAPITurnInput(input string, items []Attachment) (string, []coreapi.Attachment, error) {
	files := make([]attachments.File, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Path) == "" {
			continue
		}
		files = append(files, attachments.File{Name: strings.TrimSpace(item.Name), Path: strings.TrimSpace(item.Path), MIME: strings.TrimSpace(item.MIME), Kind: strings.TrimSpace(item.Kind)})
	}
	return attachments.PrepareInput(input, files)
}
