package govern_token

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/superconsensus-chain/xupercore/kernel/contract"
	"github.com/superconsensus-chain/xupercore/kernel/contract/proposal/utils"
	pb "github.com/superconsensus-chain/xupercore/protos"
	"math/big"
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
	register.RegisterKernMethod(utils.GovernTokenKernelContract,"Buy",t.AddTokens)
	register.RegisterKernMethod(utils.GovernTokenKernelContract,"Sell",t.SubTokens)


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

		for _, discount := range queue {
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
		}else {
			resp = fmt.Sprintf("分红奖励总量:%v", reward)
		}
		return  &contract.Response{
				Status:  utils.StatusOK,
				Message: reward.String(),
				Body:    []byte(resp),
			}, nil
	}else {
		return nil, errors.New("V__目前暂无分红信息")
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
	if takeBonus.Int64() == 0 {
		return nil, errors.New("V__缺失amount参数")
	}
	//fmt.Println("取出分红", takeBonus.Int64())

	// 读取数据库
	ledger := mgr.Ctx.RealLedger
	ctx.Args()["account"] = []byte(ctx.Initiator())
	// 到账高度
	ctx.Args()["height"] = []byte(big.NewInt(ledger.GetMeta().TrunkHeight + 3).String())
	// 先查询可供提现数量
	response, rewardE := mgr.BonusQuery(ctx)
	if rewardE != nil {
		return nil, rewardE
	}
	available, _ := big.NewInt(0).SetString(response.Message, 10)
	if available.Cmp(takeBonus) < 0 {
		return nil, errors.New("V__可供提现分红奖励不足提现数量")
	}
	return  &contract.Response{
		Status:  utils.StatusOK,
		Message: "success",
		Body:    []byte("提现成功"),
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
