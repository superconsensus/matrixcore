package govern_token

import (
	"fmt"
	"math/big"

	rledger "github.com/superconsensus/matrixcore/bcs/ledger/xledger/ledger"
	xledger "github.com/superconsensus/matrixcore/bcs/ledger/xledger/ledger"
	"github.com/superconsensus/matrixcore/kernel/common/xcontext"
	"github.com/superconsensus/matrixcore/kernel/contract"
	"github.com/superconsensus/matrixcore/kernel/contract/proposal/utils"
	"github.com/superconsensus/matrixcore/kernel/ledger"
	"github.com/superconsensus/matrixcore/lib/logs"
	"github.com/superconsensus/matrixcore/lib/timer"
)

type LedgerRely interface {
	// 从创世块获取创建合约账户消耗gas
	GetNewGovGas() (int64, error)
	// 从创世块获取创建合约账户消耗gas
	GetGenesisPreDistribution() ([]xledger.Predistribution, error)
	// 获取状态机最新确认快照
	GetTipXMSnapshotReader() (ledger.XMSnapshotReader, error)
	// 计算UTXO总量视为治理代币总量
	CalGovTokenTotal() (*big.Int, error)
}

type GovCtx struct {
	// 基础上下文
	xcontext.BaseCtx
	BcName   string
	Ledger   LedgerRely
	Contract contract.Manager
	// 增加真正的账本字段
	RealLedger *rledger.Ledger
}

func NewGovCtx(bcName string, leg LedgerRely, contract contract.Manager) (*GovCtx, error) {
	if bcName == "" || leg == nil || contract == nil {
		return nil, fmt.Errorf("new gov ctx failed because param error")
	}

	log, err := logs.NewLogger("", utils.GovernTokenKernelContract)
	if err != nil {
		return nil, fmt.Errorf("new gov ctx failed because new logger error. err:%v", err)
	}

	ctx := new(GovCtx)
	ctx.XLog = log
	ctx.Timer = timer.NewXTimer()
	ctx.BcName = bcName
	ctx.Ledger = leg
	ctx.Contract = contract

	return ctx, nil
}
