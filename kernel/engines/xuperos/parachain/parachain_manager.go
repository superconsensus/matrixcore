package parachain

import (
	"fmt"
	"strconv"

	"github.com/spf13/viper"
	"github.com/superconsensus/matrixcore/kernel/contract"
	"github.com/superconsensus/matrixcore/kernel/engines/xuperos/common"
)

const (
	ConfigName = "engine.yaml"
)

type ParaChainConfig struct {
	MinNewChainAmount string
}

// Manager
type Manager struct {
	Ctx *ParaChainCtx
}

// NewParaChainManager create instance of ParaChain
func NewParaChainManager(ctx *ParaChainCtx) (*Manager, error) {
	if ctx == nil || ctx.Contract == nil || ctx.BcName == "" {
		return nil, fmt.Errorf("parachain ctx set error")
	}
	conf, err := loadConfig(ctx.ChainCtx.EngCtx.EnvCfg.GenConfFilePath(ConfigName))
	if err != nil {
		return nil, err
	}

	minNewChainAmount, err := strconv.ParseInt(conf.MinNewChainAmount, 10, 64)
	if err != nil {
		return nil, err
	}
	t := NewParaChainContract(ctx.BcName, minNewChainAmount, ctx.ChainCtx)
	register := ctx.Contract.GetKernRegistry()
	// 注册合约方法
	kMethods := map[string]contract.KernMethod{
		// 在链名单中: group.admin/identities -> group.admin/identities/secAdmin
		"createChain": t.createChain,
		//"editGroup":   t.editGroup, // 停用此方法
		"getGroup":    t.getGroup,
		"stopChain":   t.stopChain,
		// 创链者管理管理员（参数可批量）
		"adminManage":		t.adminManage,
		// 管理员邀请加入（参数可批量）
		"inviteMembers":	t.inviteMembers,
		// 处理邀请，同意/拒绝
		"inviteHandle":		t.inviteHandle,
		// 管理员剔除成员（参数可批量）
		"removeMembers":	t.removeMembers,
		// 申请加入
		"joinApply":		t.joinApply,
		// 管理员对加入申请处理（参数可批量）
		"joinHandle":		t.joinHandle,
		// 主动退出
		"exitChain":		t.exitChain,
		// 查询历史邀请申请消息
		"getHistory":  t.getHistory,
		// 清理过期邀请/申请消息
		//"cleanExpiration":  t.cleanExpiration,
	}
	for method, f := range kMethods {
		if _, err := register.GetKernMethod(ParaChainKernelContract, method); err != nil {
			register.RegisterKernMethod(ParaChainKernelContract, method, f)
		}
	}

	// 仅主链绑定handleCreateChain 从链上下文中获取链绑定的异步任务worker
	asyncTask := map[string]common.TaskHandler{
		"CreateBlockChain":  t.handleCreateChain,
		"StopBlockChain":    t.handleStopChain,
		"RefreshBlockChain": t.handleRefreshChain,
	}
	for task, f := range asyncTask {
		ctx.ChainCtx.Asyncworker.RegisterHandler(ParaChainKernelContract, task, f)
	}
	mg := &Manager{
		Ctx: ctx,
	}

	return mg, nil
}

func loadConfig(fname string) (*ParaChainConfig, error) {
	viperObj := viper.New()
	viperObj.SetConfigFile(fname)
	err := viperObj.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("read config failed.path:%s,err:%v", fname, err)
	}

	cfg := &ParaChainConfig{
		MinNewChainAmount: "100",
	}
	if err = viperObj.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmatshal config failed.path:%s,err:%v", fname, err)
	}
	return cfg, nil
}
