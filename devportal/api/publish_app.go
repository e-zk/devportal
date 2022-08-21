package api

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/accrescent/apkstat"
	"github.com/gin-gonic/gin"
	"github.com/mattn/go-sqlite3"

	"github.com/accrescent/devportal/config"
)

func PublishApp(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)
	conf := c.MustGet("config").(*config.Config)
	ghID := c.MustGet("gh_id").(int64)
	appID := c.Param("appID")

	var appPath string
	if err := db.QueryRow(
		"SELECT path FROM submitted_apps WHERE id = ?",
		appID,
	).Scan(&appPath); err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	rawAPKSet, err := os.Open(appPath)
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	apkSet, err := io.ReadAll(rawAPKSet)
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	apkSetReader := bytes.NewReader(apkSet)
	z, err := zip.NewReader(apkSetReader, int64(len(apkSet)))
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	rawBaseAPK, err := z.Open("splits/base-master.apk")
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	defer rawBaseAPK.Close()
	baseAPK, err := io.ReadAll(rawBaseAPK)
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	apk, err := apk.FromReader(bytes.NewReader(baseAPK), int64(len(baseAPK)))
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	m := apk.Manifest()

	tx, err := db.Begin()
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	if _, err := tx.Exec("INSERT INTO app_teams (id) VALUES (?)", appID); err != nil {
		if errors.Is(err.(sqlite3.Error).ExtendedCode, sqlite3.ErrConstraintPrimaryKey) {
			_ = c.AbortWithError(http.StatusConflict, err)
		} else {
			_ = c.AbortWithError(http.StatusInternalServerError, err)
		}
		if err := tx.Rollback(); err != nil {
			_ = c.Error(err)
		}
		return
	}
	if _, err := tx.Exec(
		"INSERT INTO app_team_users (app_id, user_gh_id) VALUES (?, ?)",
		appID, ghID,
	); err != nil {
		if errors.Is(err.(sqlite3.Error).ExtendedCode, sqlite3.ErrConstraintPrimaryKey) {
			_ = c.AbortWithError(http.StatusConflict, err)
		} else {
			_ = c.AbortWithError(http.StatusInternalServerError, err)
		}
		if err := tx.Rollback(); err != nil {
			_ = c.Error(err)
		}
		return
	}
	if _, err := tx.Exec("DELETE FROM submitted_apps WHERE id = ?", appID); err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		if err := tx.Rollback(); err != nil {
			_ = c.Error(err)
		}
		return
	}

	// Publish to repository server
	req, err := http.NewRequest(
		"POST",
		fmt.Sprintf("%s/apps/%s/%d/%s", conf.RepoURL, appID, m.VersionCode, m.VersionName),
		apkSetReader,
	)
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		if err := tx.Rollback(); err != nil {
			_ = c.Error(err)
		}
		return
	}
	req.Header.Add("Authorization", "token "+conf.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		if err := tx.Rollback(); err != nil {
			_ = c.Error(err)
		}
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusBadRequest:
			c.AbortWithStatus(http.StatusInternalServerError)
		case http.StatusUnauthorized:
			_ = c.AbortWithError(
				http.StatusInternalServerError,
				errors.New("invalid repo server API key"),
			)
		case http.StatusConflict:
			_ = c.AbortWithError(resp.StatusCode, errors.New("app already published"))
		default:
			_ = c.AbortWithError(
				http.StatusInternalServerError,
				errors.New("unknown error"),
			)
		}
		if err := tx.Rollback(); err != nil {
			_ = c.Error(err)
		}
		return
	}

	if err := tx.Commit(); err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	c.String(http.StatusOK, "")
}
