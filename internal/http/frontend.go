package httpapi

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/gofiber/fiber/v2"
)

const (
	frontendIndexPath  = "web/index.html"
	frontendAssetsDir  = "web/assets"
	defaultMapboxStyle = "mapbox/light-v11"
)

//go:embed web/index.html web/assets/*
var frontendFiles embed.FS

type WebConfig struct {
	MapboxAccessToken string
	MapboxStyle       string
}

type frontendConfigPayload struct {
	MapboxAccessToken string `json:"mapboxAccessToken"`
	MapboxStyle       string `json:"mapboxStyle"`
}

func registerFrontend(app *fiber.App, webConfig WebConfig) {
	app.Get("/", serveFrontendIndex)
	app.Get("/assets/config.js", serveFrontendConfig(webConfig))
	app.Get("/assets/:file", serveFrontendAsset)
}

func webConfigOrDefault(webConfig *WebConfig) WebConfig {
	if webConfig == nil {
		return WebConfig{}
	}

	return *webConfig
}

func serveFrontendIndex(ctx *fiber.Ctx) error {
	return sendEmbeddedFile(ctx, frontendIndexPath, "text/html; charset=utf-8", "")
}

func serveFrontendAsset(ctx *fiber.Ctx) error {
	fileName := path.Clean(ctx.Params("file"))
	if fileName == "." || strings.HasPrefix(fileName, "/") || strings.Contains(fileName, "/") {
		return fiber.ErrNotFound
	}

	contentType := assetContentType(fileName)
	cacheControl := "public, max-age=3600"

	return sendEmbeddedFile(ctx, path.Join(frontendAssetsDir, fileName), contentType, cacheControl)
}

func serveFrontendConfig(webConfig WebConfig) fiber.Handler {
	if webConfig.MapboxStyle == "" {
		webConfig.MapboxStyle = defaultMapboxStyle
	}

	return func(ctx *fiber.Ctx) error {
		data, err := json.Marshal(frontendConfigPayload(webConfig))
		if err != nil {
			return fmt.Errorf("encode frontend config: %w", err)
		}

		ctx.Set(fiber.HeaderContentType, "text/javascript; charset=utf-8")
		ctx.Set(fiber.HeaderCacheControl, "no-store")

		return ctx.SendString("window.FSQR_CONFIG = " + string(data) + ";\n")
	}
}

func sendEmbeddedFile(ctx *fiber.Ctx, filePath, contentType, cacheControl string) error {
	data, err := frontendFiles.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fiber.ErrNotFound
		}

		return fmt.Errorf("read embedded frontend file %q: %w", filePath, err)
	}

	if contentType != "" {
		ctx.Set(fiber.HeaderContentType, contentType)
	}
	if cacheControl != "" {
		ctx.Set(fiber.HeaderCacheControl, cacheControl)
	}

	return ctx.Send(data)
}

func assetContentType(fileName string) string {
	switch path.Ext(fileName) {
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}
