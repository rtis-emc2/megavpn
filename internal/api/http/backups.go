package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	backupsvc "github.com/rtis-emc2/megavpn/internal/backup"
)

const backupOperationTimeout = 10 * time.Minute

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireSuperadmin(w, r); !ok {
		return
	}
	if s.backupManager == nil {
		writeErr(w, http.StatusServiceUnavailable, "backup storage is unavailable")
		return
	}
	records, err := s.backupManager.List()
	if err != nil {
		s.logPersistenceFailure("list backups", err)
		writeErr(w, http.StatusInternalServerError, "list backups failed")
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := requireSuperadmin(w, r)
	if !ok {
		return
	}
	if s.backupManager == nil {
		writeErr(w, http.StatusServiceUnavailable, "backup storage is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), backupOperationTimeout)
	defer cancel()
	record, err := s.backupManager.Create(ctx)
	if err != nil {
		s.logPersistenceFailure("create backup", err)
		writeErr(w, http.StatusInternalServerError, "backup creation failed")
		return
	}
	s.auditBestEffort(r.Context(), &authCtx.User.ID, "backup.create", "backup", &record.ID, "database backup created and verified")
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) verifyBackup(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := requireSuperadmin(w, r)
	if !ok {
		return
	}
	if s.backupManager == nil {
		writeErr(w, http.StatusServiceUnavailable, "backup storage is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), backupOperationTimeout)
	defer cancel()
	id := idParam(r)
	record, err := s.backupManager.Verify(ctx, id)
	if err != nil {
		s.logPersistenceFailure("verify backup", err, "backup_id", id)
		s.auditBestEffort(r.Context(), &authCtx.User.ID, "backup.verify_failed", "backup", &id, "database backup integrity verification failed")
		writeErr(w, http.StatusUnprocessableEntity, "backup verification failed")
		return
	}
	s.auditBestEffort(r.Context(), &authCtx.User.ID, "backup.verify", "backup", &id, "database backup checksum and archive verified")
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := requireSuperadmin(w, r)
	if !ok {
		return
	}
	if s.backupManager == nil {
		writeErr(w, http.StatusServiceUnavailable, "backup storage is unavailable")
		return
	}
	id := idParam(r)
	file, record, err := s.backupManager.Open(id)
	if err != nil {
		switch {
		case errors.Is(err, backupsvc.ErrInvalidID):
			writeErr(w, http.StatusBadRequest, "invalid backup id")
		case errors.Is(err, backupsvc.ErrIntegrity):
			s.logPersistenceFailure("download backup integrity check", err, "backup_id", id)
			s.auditBestEffort(r.Context(), &authCtx.User.ID, "backup.download_blocked", "backup", &id, "database backup download blocked by integrity verification")
			writeErr(w, http.StatusUnprocessableEntity, "backup integrity verification failed")
		case errors.Is(err, os.ErrNotExist):
			writeErr(w, http.StatusNotFound, "backup not found")
		default:
			s.logPersistenceFailure("open backup for download", err, "backup_id", id)
			writeErr(w, http.StatusInternalServerError, "backup download failed")
		}
		return
	}
	defer file.Close()
	s.auditBestEffort(r.Context(), &authCtx.User.ID, "backup.download", "backup", &id, "database backup downloaded")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, record.Filename))
	w.Header().Set("Content-Length", strconv.FormatInt(record.SizeBytes, 10))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, record.Filename, record.CreatedAt, file)
}

func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := requireSuperadmin(w, r)
	if !ok {
		return
	}
	if s.backupManager == nil {
		writeErr(w, http.StatusServiceUnavailable, "backup storage is unavailable")
		return
	}
	id := idParam(r)
	if err := s.backupManager.Delete(id); err != nil {
		switch {
		case errors.Is(err, backupsvc.ErrInvalidID):
			writeErr(w, http.StatusBadRequest, "invalid backup id")
		case errors.Is(err, os.ErrNotExist):
			writeErr(w, http.StatusNotFound, "backup not found")
		default:
			s.logPersistenceFailure("delete backup", err, "backup_id", id)
			writeErr(w, http.StatusInternalServerError, "backup delete failed")
		}
		return
	}
	s.auditBestEffort(r.Context(), &authCtx.User.ID, "backup.delete", "backup", &id, "database backup deleted")
	writeJSON(w, http.StatusOK, response{"status": "ok", "deleted_backup_id": id})
}
