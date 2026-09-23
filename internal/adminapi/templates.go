package adminapi

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

const maxTemplateSize = 64 << 20

func (s *Server) uploadTemplate(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanWrite() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxTemplateSize)
	if err := r.ParseMultipartForm(maxTemplateSize); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_upload"})
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "file_required"})
		return
	}
	defer file.Close()
	name := r.FormValue("name")
	version, err := strconv.Atoi(r.FormValue("version"))
	if name == "" || err != nil || version < 1 {
		writeJSON(w, 400, map[string]string{"error": "invalid_metadata"})
		return
	}
	dir := "/var/lib/digital-ocean-bot/templates"
	if err := os.MkdirAll(dir, 0700); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	tmp, err := os.CreateTemp(dir, "upload-*.db")
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err = io.Copy(tmp, file); err != nil {
		tmp.Close()
		writeJSON(w, 500, errorBody())
		return
	}
	tmp.Close()

	meta, err := sanaei.InspectTemplate(tmpPath)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_sqlite_template"})
		return
	}
	var id string
	err = s.DB.QueryRowContext(r.Context(), `INSERT INTO xui_database_templates(id,name,version,storage_path,sha256,size_bytes) VALUES(gen_random_uuid(),$1,$2,'pending',$3,$4) RETURNING id::text`, name, version, meta.SHA256, meta.Size).Scan(&id)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	final := filepath.Join(dir, id+".db")
	if err := os.Rename(tmpPath, final); err != nil {
		_, _ = s.DB.ExecContext(r.Context(), `DELETE FROM xui_database_templates WHERE id=$1`, id)
		writeJSON(w, 500, errorBody())
		return
	}
	_, _ = s.DB.ExecContext(r.Context(), `UPDATE xui_database_templates SET storage_path=$2 WHERE id=$1`, id, final)
	writeJSON(w, 201, map[string]any{"id": id, "sha256": meta.SHA256, "size": meta.Size})
}
