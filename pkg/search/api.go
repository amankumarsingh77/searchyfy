package search

import (
	"context"
	"github.com/amankumarsingh77/search_engine/config"
	"github.com/amankumarsingh77/search_engine/internal/query"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"strconv"
)

type SearchAPI struct {
	engine *query.QueryEngine
}

func NewSearchAPI(dbPool *pgxpool.Pool, cfg *config.QueryEngineConfig) *SearchAPI {
	engine := query.NewQueryEngine(dbPool, cfg)
	return &SearchAPI{engine: engine}
}

func (api *SearchAPI) RegisterRoutes(app *fiber.App) {
	app.Get("/search", api.searchHandler)
}

func (api *SearchAPI) searchHandler(c *fiber.Ctx) error {
	queryStr := c.Query("q", "")
	page, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(c.Query("page_size", "10"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	results, total, timeTaken, err := api.engine.Search(context.Background(), queryStr, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Search failed: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"query":         queryStr,
		"page":          page,
		"page_size":     pageSize,
		"total":         total,
		"total_pages":   (total + pageSize - 1) / pageSize,
		"results":       results,
		"response_time": timeTaken,
	})
}
