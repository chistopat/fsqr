package httpapi

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/gofiber/fiber/v2"
)

const (
	frontendIndexPath = "web/index.html"
	frontendAssetsDir = "web/assets"
)

//go:embed web/index.html web/assets/*
var frontendFiles embed.FS

func registerFrontend(app *fiber.App) {
	app.Get("/", serveFrontendIndex)
	app.Get("/assets/:file", serveFrontendAsset)
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
