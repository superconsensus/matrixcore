// 统一管理系统引擎和链运行上下文
package common

import (
	"github.com/superconsensus/matrixcore/bcs/ledger/xledger/ledger"
	"github.com/superconsensus/matrixcore/bcs/ledger/xledger/state"
	"github.com/superconsensus/matrixcore/kernel/common/xaddress"
	xconf "github.com/superconsensus/matrixcore/kernel/common/xconfig"
	xctx "github.com/superconsensus/matrixcore/kernel/common/xcontext"
	"github.com/superconsensus/matrixcore/kernel/consensus"
	"github.com/superconsensus/matrixcore/kernel/contract"
	governToken "github.com/superconsensus/matrixcore/kernel/contract/proposal/govern_token"
	"github.com/superconsensus/matrixcore/kernel/contract/proposal/propose"
	timerTask "github.com/superconsensus/matrixcore/kernel/contract/proposal/timer"
	engconf "github.com/superconsensus/matrixcore/kernel/engines/xuperos/config"
	"github.com/superconsensus/matrixcore/kernel/network"
	aclBase "github.com/superconsensus/matrixcore/kernel/permission/acl/base"
	cryptoBase "github.com/superconsensus/matrixcore/lib/crypto/client/base"
)

// 引擎运行上下文环境
type EngineCtx struct {
	// 基础上下文
	xctx.BaseCtx
	// 运行环境配置
	EnvCfg *xconf.EnvConf
	// 引擎配置
	EngCfg *engconf.EngineConf
	// 网络组件句柄
	Net network.Network
	// 链管理上下文
	ChainM ChainManager
}

// 链级别上下文，维护链级别上下文，每条平行链各有一个
type ChainCtx struct {
	// 基础上下文
	xctx.BaseCtx
	// 引擎上下文
	EngCtx *EngineCtx
	// 链名
	BCName string
	// 账本
	Ledger *ledger.Ledger
	// 状态机
	State *state.State
	// 合约
	Contract contract.Manager
	// 共识
	Consensus consensus.ConsensusInterface
	// 加密
	Crypto cryptoBase.CryptoClient
	// 权限
	Acl aclBase.AclManager
	// 治理代币
	GovernToken governToken.GovManager
	// 提案
	Proposal propose.ProposeManager
	// 定时任务
	TimerTask timerTask.TimerManager
	// 结点账户信息
	Address *xaddress.Address
	// 异步任务
	Asyncworker AsyncworkerAgent
}
