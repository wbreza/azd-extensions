package contracts

import "github.com/wbreza/azd-extensions/sdk/common"

type KustomizeConfig struct {
	Directory common.ExpandableString            `yaml:"dir"`
	Edits     []common.ExpandableString          `yaml:"edits"`
	Env       map[string]common.ExpandableString `yaml:"env"`
}
