package govern_token

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/golang/protobuf/proto"
	"github.com/shopspring/decimal"
	"github.com/superconsensus/matrixcore/kernel/contract"
	"github.com/superconsensus/matrixcore/kernel/contract/proposal/utils"
	pb "github.com/superconsensus/matrixcore/protos"
)

// Manager manages all gov releated data, providing read/write interface
type Manager struct {
	Ctx *GovCtx
}

// NewGovManager create instance of GovManager
func NewGovManager(ctx *GovCtx) (GovManager, error) {
	if ctx == nil || ctx.Ledger == nil || ctx.Contract == nil || ctx.BcName == "" {
		return nil, fmt.Errorf("acl ctx set error")
	}

	newGovGas, err := ctx.Ledger.GetNewGovGas()
	if err != nil {
		return nil, fmt.Errorf("get gov gas failed.err:%v", err)
	}

	predistribution, err := ctx.Ledger.GetGenesisPreDistribution()
	if err != nil {
		return nil, fmt.Errorf("get predistribution failed.err:%v", err)
	}

	t := NewKernContractMethod(ctx.BcName, newGovGas, predistribution)
	register := ctx.Contract.GetKernRegistry()
	//register.RegisterKernMethod(utils.GovernTokenKernelContract, "Init", t.InitGovernTokens)
	//register.RegisterKernMethod(utils.GovernTokenKernelContract, "Transfer", t.TransferGovernTokens) //屏蔽掉
	register.RegisterKernMethod(utils.GovernTokenKernelContract, "Lock", t.LockGovernTokens)
	register.RegisterKernMethod(utils.GovernTokenKernelContract, "UnLock", t.UnLockGovernTokens)
	register.RegisterKernMethod(utils.GovernTokenKernelContract, "Query", t.QueryAccountGovernTokens)
	register.RegisterKernMethod(utils.GovernTokenKernelContract, "TotalSupply", t.TotalSupply)
	register.RegisterKernMethod(utils.GovernTokenKernelContract, "Buy", t.AddTokens)
	register.RegisterKernMethod(utils.GovernTokenKernelContract, "Sell", t.SubTokens)

	mg := &Manager{
		Ctx: ctx,
	}
	register.RegisterKernMethod(utils.GovernTokenKernelContract, "AllToken", mg.AllTokens)
	register.RegisterKernMethod(utils.GovernTokenKernelContract, "BonusObtain", mg.BonusObtain)
	register.RegisterKernMethod(utils.GovernTokenKernelContract, "BonusQuery", mg.BonusQuery)

	return mg, nil
}

// 获取全网UTXO总量视为治理代币总量，这个方法只提供给提案proposal到截止投票高度时计算赞成票数比调用
func (mgr *Manager) AllTokens(ctx contract.KContext) (*contract.Response, error) {
	//fmt.Println("total supply", string(ctx.Args()["stopHeight"]))
	total, err := mgr.Ctx.Ledger.CalGovTokenTotal()
	if err != nil {
		return nil, err
	}
	//fmt.Println("全网治理代币总量", total.Int64())
	return &contract.Response{
		Status:  utils.StatusOK,
		Message: "success",
		Body:    []byte(total.String()),
	}, nil
}

// 查询总计分红
func (mgr *Manager) BonusQuery(ctx contract.KContext) (*contract.Response, error) {
	args := ctx.Args()
	account := string(args["account"])
	ledger := mgr.Ctx.RealLedger
	bonusData := &pb.AllBonusData{}
	allBonusDataBytes, err := ledger.ConfirmedTable.Get([]byte("all_bonus_data"))
	if err == nil {
		pe := proto.Unmarshal(allBonusDataBytes, bonusData)
		if pe != nil {
			mgr.Ctx.XLog.Warn("V__查询分红数据反序列化出错", pe)
			return nil, pe
		}
		// 所有分红池
		pools := bonusData.BonusPools
		// 提现队列
		queue := bonusData.DiscountQueue
		// 所有分红池子奖励总和
		reward := big.NewInt(0)
		// 提现队列中因冻结而尚未到账的分红奖励
		frozen := big.NewInt(0)

		// 测试用，查看各单个池子的分红
		//bonusEveryPools := make(map[string]*big.Int)

		for _, pool := range pools { // miner, pool := range pools
			// 分红 = 票数 * 每票奖励 - 债务
			voter, ok := pool.Voters[account]
			// 测试用，本池子的分红
			//thisPoolBonus := big.NewInt(0)
			if ok {
				// 票数
				thisPoolVotes, _ := big.NewInt(0).SetString(voter.Amount, 10)
				// 每票奖励
				bonusPerVote, _ := big.NewInt(0).SetString(pool.BonusPerVote, 10)
				// 债务
				debt, _ := big.NewInt(0).SetString(voter.Debt, 10)
				thisPoolVotes.Mul(thisPoolVotes, bonusPerVote).Sub(thisPoolVotes, debt)
				// 测试用，记录本池子的分红
				//bonusEveryPools[miner] = thisPoolVotes
				reward.Add(reward, thisPoolVotes)
			}
		}
		//fmt.Println("V__测试校验查询分红", bonusEveryPools)

		for frozenHeight, discount := range queue {
			if string(args["show"]) == "yes" {
				fmt.Println("提现高度", frozenHeight, "提现情况", discount.UserDiscount)
			}
			value, ok := discount.UserDiscount[account]
			if ok {
				// 提现数量
				amount, _ := big.NewInt(0).SetString(value, 10)
				frozen.Add(frozen, amount)
			}
		}
		allReward := big.NewInt(0).Add(reward, frozen)
		var resp string
		if frozen.Int64() != 0 {
			resp = fmt.Sprintf("分红奖励总量:%v, 其中包含因冻结尚未到账的数量为:%v", allReward, frozen)
		} else {
			resp = fmt.Sprintf("分红奖励总量:%v", reward)
		}
		return &contract.Response{
			Status:  utils.StatusOK,
			Message: reward.String(),
			Body:    []byte(resp),
		}, nil
	} else {
		return nil, errors.New("V__目前暂无分红信息")
	}

}

// 同步时校验分红可用余额专用
func (mgr *Manager) SyncCheckBonus(ctx contract.KContext, height int64) (*big.Int, error) {
	args := ctx.Args()
	account := string(args["account"])
	ledger := mgr.Ctx.RealLedger
	bonusData := &pb.AllBonusData{}
	allBonusDataBytes, err := ledger.ConfirmedTable.Get([]byte("all_bonus_data"))
	if err == nil {
		pe := proto.Unmarshal(allBonusDataBytes, bonusData)
		if pe != nil {
			mgr.Ctx.XLog.Warn("V__查询分红数据反序列化出错", pe)
			return nil, pe
		}
		// 所有分红池
		pools := bonusData.BonusPools
		// 提现队列
		queue := bonusData.DiscountQueue
		// 所有分红池子奖励总和
		reward := big.NewInt(0)

		for _, pool := range pools { // miner, pool := range pools
			// 分红 = 票数 * 每票奖励 - 债务
			voter, ok := pool.Voters[account]
			if ok {
				// 票数
				thisPoolVotes, _ := big.NewInt(0).SetString(voter.Amount, 10)
				// 每票奖励
				bonusPerVote, _ := big.NewInt(0).SetString(pool.BonusPerVote, 10)
				// 债务
				debt, _ := big.NewInt(0).SetString(voter.Debt, 10)
				thisPoolVotes.Mul(thisPoolVotes, bonusPerVote).Sub(thisPoolVotes, debt)
				// 测试用，记录本池子的分红
				reward.Add(reward, thisPoolVotes)
			}
		}

		flag := false // 标志位，检查是否真的是在同步，如果是，下面的循环中frozenHeight==height至少要成立一次，否则就是传入高度有问题
		for frozenHeight, discount := range queue {
			//fmt.Println("同步查询分红，高度", frozenHeight, "提现情况", discount.UserDiscount)
			value, ok := discount.UserDiscount[account]
			if ok && frozenHeight <= height { // == height
				flag = true
				// 提现数量
				amount, _ := big.NewInt(0).SetString(value, 10)
				// 提现队列中因冻结而尚未到账的分红奖励，在同步的时候要加回去
				reward.Add(reward, amount)
			}
		}
		if flag {
			return reward, nil
		} else {
			//fmt.Println("非同步", "请求中的高度参数", height, "当前高度", ledger.GetMeta().TrunkHeight)
			mgr.Ctx.XLog.Warn("V__非同步，提现高度出错","计算奖励", reward, "请求中的高度参数", height, "当前高度", ledger.GetMeta().TrunkHeight)
			mgr.Ctx.XLog.Warn("V__分红提现表", "discountQueue", queue, "bonusPools", pools)
			return nil, errors.New("V__操作异常，请刷新页面后重试")
		}
	} else {
		return big.NewInt(0), errors.New("V__目前暂无分红信息")
	}
}

// 取出分红
// 这两个方法理论上也是可以在manager初始化完成之后在meta那边注册的，在那边注册的话同样可以直接拿到账本数据
func (mgr *Manager) BonusObtain(ctx contract.KContext) (*contract.Response, error) {
	// 校验参数
	args := ctx.Args()
	// 借用tx.args，在账本中修改数据，最后具体的到账交易在矿工打包到对应高度时生成
	amount := args["amount"]
	// 提现数量
	takeBonus, succ := big.NewInt(0).SetString(string(amount), 10)
	if !succ {
		return nil, errors.New("V__amount参数格式错误")
	}
	if takeBonus == nil {
		return nil, errors.New("V__缺失amount参数")
	}
	ledger := mgr.Ctx.RealLedger
	nowHeight := ledger.GetMeta().GetTrunkHeight()

	ctx.Args()["account"] = []byte(ctx.Initiator())
	// height从命令中传过来，实际写账本不依赖这个height了，但为了防止双花和判断同步这个参数又是必要的
	realHeight, _ := big.NewInt(0).SetString(string(args["height"]), 10)
	if realHeight == nil {
		return nil, errors.New("V__缺失height参数")
	}
	//fmt.Println("---分红提现请求高度---realHeight-2", realHeight.Int64()-2, "##取出分红", takeBonus.Int64(), "合约账本获取当前高度nowHeight", nowHeight) // 实际上当前网络高度为realHeight.Int64()-2
	// 先查询可供提现数量
	response, rewardE := mgr.BonusQuery(ctx)
	if rewardE != nil {
		return nil, rewardE
	}
	available, _ := big.NewInt(0).SetString(response.Message, 10)
	if realHeight.Int64()-nowHeight == 2 {
		// 正常交易处理
		//fmt.Println("##可用数量", available.Int64())
		mgr.Ctx.XLog.Info("V__分红提现", "realHeight", realHeight, "nowHeight", nowHeight)
		if available.Cmp(takeBonus) < 0 {
			deciAvail := decimal.NewFromInt(available.Int64())
			point := decimal.NewFromInt(100000000)
			deciAvail = deciAvail.DivRound(point, 8)
			deciTake := decimal.NewFromInt(takeBonus.Int64())
			deciTake = deciTake.DivRound(point, 8)
			availFloat, _ := deciAvail.Float64()
			takeFloat, _ := deciTake.Float64()
			return nil, fmt.Errorf("V__可供提现分红奖励不足提现数量，可用数量:%.8f,提现数量:%.8f", availFloat, takeFloat)
		}
	} else if realHeight.Int64()-nowHeight == 1 {
		// 在同步
		//fmt.Println("正常执行计算可用数量", available)
		mgr.Ctx.XLog.Info("V__sync check bonus", "realHeight", realHeight, "nowHeight", nowHeight)
		syncAvailable, syncErr := mgr.SyncCheckBonus(ctx, realHeight.Int64())
		if syncErr != nil {
			return nil, syncErr
		}
		//fmt.Println("同步时的计算可用数量", syncAvailable)
		if syncAvailable.Cmp(takeBonus) < 0 {
			deciAvail := decimal.NewFromInt(syncAvailable.Int64())
			point := decimal.NewFromInt(100000000)
			deciAvail = deciAvail.DivRound(point, 8)
			deciTake := decimal.NewFromInt(takeBonus.Int64())
			deciTake = deciTake.DivRound(point, 8)
			availFloat, _ := deciAvail.Float64()
			takeFloat, _ := deciTake.Float64()
			return nil, fmt.Errorf("V__可供提现分红奖励不足提现数量，可用数量:%.8f,提现数量:%.8f", availFloat, takeFloat)
		}
	} else {
		// 高度参数错误，大概率是因为同步块中的提现交易出现了延迟，使用syncCheck成功通过，否则失败
		syncAvailable, syncErr := mgr.SyncCheckBonus(ctx, nowHeight)
		if syncErr != nil {
			return nil, syncErr
		}
		if syncAvailable.Cmp(takeBonus) < 0 {
			mgr.Ctx.XLog.Warn("V__提现高度出错", "请求中的高度参数", realHeight.Int64(), "当前高度", nowHeight)
			//fmt.Println("提现高度出错", "请求中的高度参数", realHeight.Int64(), "当前高度", nowHeight)
			return nil, errors.New("V__操作异常，请刷新页面后重试")
		}
	}
	ctx.Args()["height"] = args["height"]

	// Key
	obtainKey := fmt.Sprintf("bonus_obtain_%s", ctx.Initiator())

	/*// 快照-5，三个块之前的快照数据
	historyHeight := big.NewInt(0)
	blk, err := mgr.Ctx.Ledger.QueryBlockByHeight(realHeight.Int64() - 5)
	if err != nil {
		mgr.Ctx.XLog.Warn("V__查询块失败", "查询高度", realHeight.Int64()-5, "err", err)
		return nil, err
	}
	reader, err := mgr.Ctx.Ledger.CreateSnapshot(blk.GetBlockid())
	if err != nil {
		mgr.Ctx.XLog.Warn("V__根据块创建快照失败", "blkId", blk.GetBlockid(), "err", err)
		return nil, err
	}
	versionData, err := reader.Get("$bonus", []byte(obtainKey))
	if err != nil {
		mgr.Ctx.XLog.Warn("V__快照reader查询key失败","key(string)", obtainKey, "err", err)
		return nil, err
	}
	if versionData == nil || versionData.PureData == nil {
		mgr.Ctx.XLog.Warn("V__查询快照结果数据为空")
		return nil, errors.New("查询结果数据为空")
	}
	historyHeight.SetString(string(versionData.PureData.Value), 10)
	//fmt.Println("距当前网络高度3个块前的快照所记录的提现高度", historyHeight)

	//判断传进来的高度与3个块前的快照的保存高度
	if realHeight.Int64() - historyHeight.Int64() < 3 {
		mgr.Ctx.XLog.Warn("V__提现请求高度需要大于快照记录的高度", "命令行或请求高度-2", realHeight.Int64()-2, "快照记录的高度-2", historyHeight.Int64()-2)
		return nil, errors.New("提现请求高度需要大于快照记录的高度")
	}*/

	// 校验重复提现，利用合约bucketKV和读写集防止提现双花
	initiatorBytes, err := ctx.Get("$bonus", []byte(obtainKey))
	if err == nil {
		// 最后一次提现的高度
		oldHeight, _ := big.NewInt(0).SetString(string(initiatorBytes), 10)
		lastHeight := oldHeight.Int64()
		if realHeight.Int64() - lastHeight == 0 {
			return nil, errors.New("V__操作过于频繁，请稍后重试")
		} else if lastHeight > realHeight.Int64() {
			return nil, errors.New("V__交易请求超过当前区块高度，无法处理")
		}
	}
	// 不为nil说明是历史第一次提现，直接put；为nil判断不是同一块内的提现操作也可以put
	ctx.Put("$bonus", []byte(obtainKey), []byte(realHeight.String()))
	// 读写集
	//rwSet := ctx.RWSet()
	//fmt.Println("bonusR", string(rwSet.RSet[0].PureData.GetKey()), string(rwSet.RSet[0].PureData.GetValue()))
	//fmt.Println("bonusW", string(rwSet.WSet[0].GetKey()), string(rwSet.WSet[0].GetValue()))

	// 上下文增加gas参数
	newGovGas, err := mgr.Ctx.Ledger.GetNewGovGas()
	if err != nil {
		return nil, fmt.Errorf("get gov gas failed.err:%v", err)
	}
	delta := contract.Limits{
		XFee: newGovGas,
	}
	ctx.AddResourceUsed(delta)

	return &contract.Response{
		Status:  utils.StatusOK,
		Message: "success",
		Body:    nil,
	}, nil
}

// GetGovTokenBalance get govern token balance of an account
func (mgr *Manager) GetGovTokenBalance(accountName string) (*pb.GovernTokenBalance, error) {
	accountBalanceBuf, err := mgr.GetObjectBySnapshot(utils.GetGovernTokenBucket(), []byte(utils.MakeAccountBalanceKey(accountName)))
	if err != nil {
		return nil, fmt.Errorf("query account balance failed.err:%v", err)
	}

	balance := utils.NewGovernTokenBalance()
	err = json.Unmarshal(accountBalanceBuf, balance)
	if err != nil {
		return nil, fmt.Errorf("no sender found")
	}

	balanceRes := &pb.GovernTokenBalance{
		TotalBalance: balance.TotalBalance.String(),
	}

	return balanceRes, nil
}

// DetermineGovTokenIfInitialized
func (mgr *Manager) DetermineGovTokenIfInitialized() (bool, error) {
	res, err := mgr.GetObjectBySnapshot(utils.GetGovernTokenBucket(), []byte(utils.GetDistributedKey()))
	if err != nil {
		return false, fmt.Errorf("query govern if initialized failed, err:%v", err)
	}

	if string(res) == "true" {
		return true, nil
	}

	return false, nil
}

func (mgr *Manager) GetObjectBySnapshot(bucket string, object []byte) ([]byte, error) {
	// 根据tip blockid 创建快照
	reader, err := mgr.Ctx.Ledger.GetTipXMSnapshotReader()
	if err != nil {
		return nil, err
	}

	return reader.Get(bucket, object)
}
