package xpoa

import (
	"encoding/json"
	"time"

	chainedBft "github.com/xuperchain/xupercore/kernel/consensus/base/driver/chained-bft"
)

// xpoaStatus 实现了ConsensusStatus接口
type XpoaStatus struct {
	Version     int64 `json:"version"`
	StartHeight int64 `json:"startHeight"`
	Index       int   `json:"index"`
	election    *xpoaSchedule
}

// 获取共识版本号
func (x *XpoaStatus) GetVersion() int64 {
	return x.Version
}

// 共识起始高度
func (x *XpoaStatus) GetConsensusBeginInfo() int64 {
	return x.StartHeight
}

// 获取共识item所在consensus slice中的index
func (x *XpoaStatus) GetStepConsensusIndex() int {
	return x.Index
}

// 获取共识类型
func (x *XpoaStatus) GetConsensusName() string {
	return "xpoa"
}

// 获取当前状态机term
func (x *XpoaStatus) GetCurrentTerm() int64 {
	term, _, _ := x.election.minerScheduling(time.Now().UnixNano(), len(x.election.validators))
	return term
}

// 获取当前矿工信息
func (x *XpoaStatus) GetCurrentValidatorsInfo() []byte {
	var v []*chainedBft.ProposerInfo
	for _, a := range x.election.validators {
		v = append(v, &chainedBft.ProposerInfo{
			Address: a,
			Neturl:  x.election.addrToNet[a],
		})
	}
	i := ValidatorsInfo{
		Validators: v,
	}
	b, _ := json.Marshal(i)
	return b
}

type ValidatorsInfo struct {
	Validators []*chainedBft.ProposerInfo `json:"validators"`
}