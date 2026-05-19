package core

import (
	"fmt"
	"time"

	"yuelaiengine/gateway/internal/config"
)

type configVersionMeta struct {
	Version   string    `json:"version"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

func (g *Gateway) recordConfigSnapshotLocked(cfg *config.GatewayConfig, source string) {
	if cfg == nil {
		return
	}
	snapshot := configSnapshot{
		Version:   g.nextVersionID(),
		Source:    source,
		CreatedAt: time.Now(),
		Config:    cfg,
	}
	g.configHistory = append(g.configHistory, snapshot)
	if g.maxHistory <= 0 {
		g.maxHistory = 20
	}
	if len(g.configHistory) > g.maxHistory {
		g.configHistory = append([]configSnapshot(nil), g.configHistory[len(g.configHistory)-g.maxHistory:]...)
	}
}

func (g *Gateway) currentConfigVersionLocked() *configVersionMeta {
	if len(g.configHistory) == 0 {
		return nil
	}
	last := g.configHistory[len(g.configHistory)-1]
	return &configVersionMeta{
		Version:   last.Version,
		Source:    last.Source,
		CreatedAt: last.CreatedAt,
	}
}

func (g *Gateway) configVersionListLocked() []configVersionMeta {
	out := make([]configVersionMeta, 0, len(g.configHistory))
	for _, item := range g.configHistory {
		out = append(out, configVersionMeta{
			Version:   item.Version,
			Source:    item.Source,
			CreatedAt: item.CreatedAt,
		})
	}
	return out
}

func (g *Gateway) rollbackToVersionLocked(version string) error {
	if version == "" {
		return fmt.Errorf("version 不能为空")
	}
	for i := len(g.configHistory) - 1; i >= 0; i-- {
		item := g.configHistory[i]
		if item.Version == version {
			if item.Config == nil {
				return fmt.Errorf("版本 %s 没有可用配置快照", version)
			}
			return g.applyConfigLocked(item.Config.Clone(), "rollback:"+version)
		}
	}
	return fmt.Errorf("版本 %s 不存在", version)
}
