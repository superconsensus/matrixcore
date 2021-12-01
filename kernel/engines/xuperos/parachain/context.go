package parachain

import (
	"fmt"

	"github.com/superconsensus/matrixcore/kernel/common/xcontext"
	"github.com/superconsensus/matrixcore/kernel/contract"
	"github.com/superconsensus/matrixcore/kernel/engines/xuperos/common"
	"github.com/superconsensus/matrixcore/lib/logs"
	"github.com/superconsensus/matrixcore/lib/timer"
)

const (
	ParaChainKernelContract = "$parachain"
)

type ParaChainCtx struct {
	// 基础上下文
	xcontext.BaseCtx
	BcName   string
	Contract contract.Manager
	ChainCtx *common.ChainCtx
}

func NewParaChainCtx(bcName string, cctx *common.ChainCtx) (*ParaChainCtx, error) {
	if bcName == "" || cctx == nil {
		return nil, fmt.Errorf("new parachain ctx failed because param error")
	}

	log, err := logs.NewLogger("", ParaChainKernelContract)
	if err != nil {
		return nil, fmt.Errorf("new parachain ctx failed because new logger error. err:%v", err)
	}

	ctx := new(ParaChainCtx)
	ctx.XLog = log
	ctx.Timer = timer.NewXTimer()
	ctx.BcName = bcName
	ctx.Contract = cctx.Contract
	ctx.ChainCtx = cctx

	return ctx, nil
}
