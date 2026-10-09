package validation

import "net/http"

type FileInspector struct{}

func (FileInspector) MIME(prefix []byte) string { return http.DetectContentType(prefix) }
