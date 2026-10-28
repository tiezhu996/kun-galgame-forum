package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"kun-galgame-api/internal/friendlink/healthcheck"
	"kun-galgame-api/internal/friendlink/siteprobe"
	"kun-galgame-api/internal/infrastructure/database"
	"kun-galgame-api/pkg/config"
	"kun-galgame-api/pkg/logger"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("加载配置失败", "error", err)
		os.Exit(1)
	}
	logger.Init(cfg.Server.Mode)
	db := database.NewPostgres(cfg.Database, cfg.Server.Mode)

	stats, err := healthcheck.Run(context.Background(), db, siteprobe.NewHTTP(0), healthcheck.DefaultConcurrency)
	if err != nil {
		slog.Error("友链健康检查失败", "error", err)
		os.Exit(1)
	}
	slog.Info("友链健康检查完成",
		"checked", stats.Checked, "failed", stats.Failed,
		"marked", stats.Marked, "recovered", stats.Recovered, "errored", stats.Errored)
	fmt.Printf("完成: 检查 %d, 失败 %d, 新异常 %d, 已恢复 %d, 写入错误 %d。\n",
		stats.Checked, stats.Failed, stats.Marked, stats.Recovered, stats.Errored)
}
