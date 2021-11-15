package tdpos

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	common "github.com/superconsensus-chain/xupercore/kernel/consensus/base/common"
	"github.com/superconsensus-chain/xupercore/kernel/contract/proposal/utils"

	"github.com/superconsensus-chain/xupercore/kernel/contract"
)

// 本文件实现tdpos的原Run方法，现全部移至三代合约
// tdpos原有的9个db存储现转化为一个bucket，4个key的链上存储形式
// contractBucket = "$tdpos"/ "$xpos"
// 1. 候选人提名相关key = "nominate"
//                value = <${candi_addr}, <${from_addr}, ${ballot_count}>>
// 2. 投票候选人相关key = "vote_${candi_addr}"
//                value = <${from_addr}, ${ballot_count}>
// 3. 撤销动作相关  key = "revoke_${candi_addr}"
//                value = <${from_addr}, <(${TYPE_VOTE/TYPE_NOMINATE}, ${ballot_count})>>
// 以上所有的数据读通过快照读取, 快照读取的是当前区块的前三个区块的值
// 以上所有数据都更新到各自的链上存储中，直接走三代合约写入，去除原Finalize的最后写入更新机制
// 由于三代合约读写集限制，不能针对同一个ExeInput触发并行操作，后到的tx将会出现读写集错误，即针对同一个大key的操作同一个区块只能顺序执行
// 撤销走的是proposal合约，但目前看来proposal没有指明height

// runNominateCandidate 执行提名候选人
func (tp *tdposConsensus) runNominateCandidate(contractCtx contract.KContext) (*contract.Response, error) {
	// 1.1 核查nominate合约参数有效性
	candidateName, height, err := tp.checkArgs(contractCtx.Args())
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	////1.1.1 核查分红比的参数有效性
	//_, err = tp.checkRatio(contractCtx.Args())
	//if err != nil {
	//	return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	//}
	amountBytes := contractCtx.Args()["amount"]
	amountStr := string(amountBytes)
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if amount <= 0 || err != nil {
		return common.NewContractErrResponse(common.StatusErr, amountErr.Error()), amountErr
	}
	// 1.2 是否按照要求多签
	if ok := tp.isAuthAddress(candidateName, contractCtx.Initiator(), contractCtx.AuthRequire()); !ok {
		return common.NewContractErrResponse(common.StatusErr, authErr.Error()), authErr
	}
	// 1.3 调用冻结接口
	tokenArgs := map[string][]byte{
		"from":      []byte(contractCtx.Initiator()),
		"amount":    []byte(fmt.Sprintf("%d", amount)),
		"lock_type": []byte(utils.GovernTokenTypeTDPOS),
	//	"ratio" : []byte(fmt.Sprintf("%d",ratio)),
	//	"to": []byte(candidateName),
	}
	_, err = contractCtx.Call("xkernel", utils.GovernTokenKernelContract, "Lock", tokenArgs)
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), errors.New("提名失败，治理代币余额不足")
	}

	// 2. 读取提名候选人key
	nKey := fmt.Sprintf("%s_%d_%s", tp.status.Name, tp.status.Version, nominateKey)
	res, err := tp.election.getSnapshotKey(height, tp.election.bindContractBucket, []byte(nKey))
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, "Internal error."), err
	}
	nominateValue := NewNominateValue()
	if res != nil { // 非首次初始化
		if err := json.Unmarshal(res, &nominateValue); err != nil {
			tp.log.Error("tdpos::runNominateCandidate::load read set err.")
			return common.NewContractErrResponse(common.StatusErr, "Internal error."), err
		}
	}
	if _, ok := nominateValue[candidateName]; ok { // 已经提过名
		return common.NewContractErrResponse(common.StatusErr, repeatNominateErr.Error()), repeatNominateErr
	}
	record := make(map[string]int64)
	record[contractCtx.Initiator()] = amount
	nominateValue[candidateName] = record

	// 3. 候选人改写
	returnBytes, err := json.Marshal(nominateValue)
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	if err := contractCtx.Put(tp.election.bindContractBucket, []byte(nKey), returnBytes); err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	if err := contractCtx.Flush(); err != nil {
		//fmt.Println("flush error", err.Error())
		tp.log.Error("flush error", err.Error())
	}
	rwSet := contractCtx.RWSet() // 读写集
	// set[0] tdpos_0_nominate; set[1] balanceOf_SmJG3rH2ZzYQ9ojxhbRCPwFiE9y6pD1Co
	if rwSet.RSet[0].PureData.GetValue() != nil {
		tempNominateByte := rwSet.RSet[0].PureData.GetValue() // 从读集获取最新数据
		tempNominateValue := NewNominateValue()
		if err := json.Unmarshal(tempNominateByte, &tempNominateValue); err != nil { // remind &
			//fmt.Println("V_unmarshal error", err.Error())
			tp.log.Error("V__提名表反序列化失败", err.Error(), "bytes", tempNominateByte)
		}
		// 校验高度可以根据实际情况修改
		if _, ok := tempNominateValue[candidateName]; ok && height >= 1 { // 校验重复提名，校验目标为第一个块之后
			return common.NewContractErrResponse(common.StatusErr, "Msg_目标已经被提过名"), errors.New("Error_目标已经被提过名")
			//fmt.Println("Error_目标已经被提过名") // 实际上应该return的，但是这样后加入的节点同步错误块时又会崩溃，，
		}
		// 提名时冻结的金额
		record := make(map[string]int64)
		record[contractCtx.Initiator()] = amount
		tempNominateValue[candidateName] = record
		returnBytes, err := json.Marshal(tempNominateValue)
		if err != nil {
			return common.NewContractErrResponse(common.StatusErr, err.Error()), err
		}
		// 反序列化后写回
		if err := contractCtx.Put(tp.election.bindContractBucket, []byte(nKey), returnBytes); err != nil {
			return common.NewContractErrResponse(common.StatusErr, err.Error()), err
		}
	}
	delta := contract.Limits{
		XFee: fee,
	}
	contractCtx.AddResourceUsed(delta)
	return common.NewContractOKResponse([]byte("ok")), nil
}

// runRevokeCandidate 执行候选人撤销,仅支持自我撤销
// 重构后的候选人撤销
// Args: candidate::候选人钱包地址
func (tp *tdposConsensus) runRevokeCandidate(contractCtx contract.KContext) (*contract.Response, error) {
	// 核查撤销nominate合约参数有效性
	candidateName, height, err := tp.checkArgs(contractCtx.Args())
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	// 1. 提名候选人改写
	nKey := fmt.Sprintf("%s_%d_%s", tp.status.Name, tp.status.Version, nominateKey)
	res, err := tp.election.getSnapshotKey(height, tp.election.bindContractBucket, []byte(nKey))
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, "Internal error."), err
	}
	nominateValue := NewNominateValue()
	if res != nil {
		if err := json.Unmarshal(res, &nominateValue); err != nil {
			tp.log.Error("tdpos::runRevokeCandidate::load read set err.")
			return common.NewContractErrResponse(common.StatusErr, notFoundErr.Error()), err
		}
	}
	// 1.1 查看是否有历史投票
	v, ok := nominateValue[candidateName]
	if !ok {
		return common.NewContractErrResponse(common.StatusErr, emptyNominateKey.Error()), emptyNominateKey
	}
	ballot, ok := v[contractCtx.Initiator()]
	if !ok {
		return common.NewContractErrResponse(common.StatusErr, notFoundErr.Error()), notFoundErr
	}
	// 1.2 查询到amount之后，再调用解冻接口，Args: FromAddr, amount
	tokenArgs := map[string][]byte{
		"from":      []byte(contractCtx.Initiator()),
		"amount":    []byte(fmt.Sprintf("%d", ballot)),
		"lock_type": []byte(utils.GovernTokenTypeTDPOS),
	}
	_, err = contractCtx.Call("xkernel", utils.GovernTokenKernelContract, "UnLock", tokenArgs)
	if err != nil {
		//return common.NewContractErrResponse(common.StatusErr, err.Error()), errors.New("撤销提名失败，（治理）余额不足")
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}

	// 2. 读取撤销记录
	rKey := fmt.Sprintf("%s_%d_%s", tp.status.Name, tp.status.Version, revokeKey)
	res, err = tp.election.getSnapshotKey(height, tp.election.bindContractBucket, []byte(rKey))
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, "Internal error."), err
	}
	revokeValue := NewRevokeValue()
	if res != nil {
		if err := json.Unmarshal(res, &revokeValue); err != nil {
			tp.log.Error("tdpos::runRevokeCandidate::load revoke read set err.")
			return common.NewContractErrResponse(common.StatusErr, notFoundErr.Error()), err
		}
	}

	// 3. 更改撤销记录
	if _, ok := revokeValue[contractCtx.Initiator()]; !ok {
		revokeValue[contractCtx.Initiator()] = make([]revokeItem, 0)
	}
	revokeValue[contractCtx.Initiator()] = append(revokeValue[contractCtx.Initiator()], revokeItem{
		RevokeType:    NOMINATETYPE,
		Ballot:        ballot,
		TargetAddress: candidateName,
	})
	revokeBytes, err := json.Marshal(revokeValue)
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}

	// 4. 删除候选人记录
	delete(nominateValue, candidateName)
	nominateBytes, err := json.Marshal(nominateValue)
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	if err := contractCtx.Put(tp.election.bindContractBucket, []byte(rKey), revokeBytes); err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	if err := contractCtx.Put(tp.election.bindContractBucket, []byte(nKey), nominateBytes); err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	if err := contractCtx.Flush(); err != nil {
		//fmt.Println("flush error", err.Error())
		tp.log.Error("flush error", err.Error())
	}
	rwSet := contractCtx.RWSet() // 读写集
	// [0] tdpos_0_nominate; [1] tdpos_0_revoke; [2] balanceOf_SmJG3rH2ZzYQ9ojxhbRCPwFiE9y6pD1Co
	//tp.log.Info("rset key", string(rwSet.RSet[0].PureData.GetKey()), "value", string(rwSet.RSet[0].PureData.GetValue()))
	//fmt.Println("rset", string(rwSet.RSet[0].PureData.GetKey()), string(rwSet.RSet[0].PureData.GetValue()))
	//fmt.Println("rset", string(rwSet.WSet[0].GetKey()), string(rwSet.WSet[0].GetValue()))
	//fmt.Println("rset", string(rwSet.RSet[1].PureData.GetKey()), string(rwSet.RSet[1].PureData.GetValue()))
	//fmt.Println("rset", string(rwSet.RSet[2].PureData.GetKey()), string(rwSet.RSet[2].PureData.GetValue()))
	//&& !bytes.Equal(rwSet.RSet[0].PureData.GetValue(), rwSet.WSet[0].GetValue())
	// 兼容旧版本已经上链的交易，用高度过滤本判断，如果链从零开始运行则可忽略高度条件
	if ( rwSet.RSet[0].PureData.GetValue() != nil || rwSet.RSet[1].PureData.GetValue() != nil ) && height > 1920000{
		// Set[0] --> nominate table
		// Set[1] --> revoke table add info
		// Set[2] --> 被撤销提名的用户治理代币balance
		// 更新提名表
		tempNominateByte := rwSet.RSet[0].PureData.GetValue() // 获取最新数据
		tempNominateValue := NewNominateValue()
		if err := json.Unmarshal(tempNominateByte, &tempNominateValue); err != nil {
			//fmt.Println("V_unmarshal error", err.Error())
			tp.log.Error("V__撤销提名-提名表反序列化失败", err.Error(), "bytes", tempNominateByte)
		}
		//tp.log.Info("tempNominateValue", tempNominateValue)
		//fmt.Println("tempNominateValue", tempNominateValue)
		// 反序列化后得到map，校验对象是否在map中——必要，因为外层的校验读取的不是最新的数据——另外还可能要校验输入amount与表中数据一致问题
		_, ok := tempNominateValue[candidateName]
		if !ok {
			return common.NewContractErrResponse(common.StatusErr, "撤销对象已不在候选列表中"), errors.New("撤销对象已不在候选列表中")
		}
		// 存在，将撤销对象从中删除
		delete(tempNominateValue, candidateName)
		// 再序列化写回
		tempNominateBytes, err := json.Marshal(tempNominateValue)
		if err != nil {
			return common.NewContractErrResponse(common.StatusErr, err.Error()), err
		}
		if err := contractCtx.Put(tp.election.bindContractBucket, []byte(nKey), tempNominateBytes); err != nil {
			return common.NewContractErrResponse(common.StatusErr, err.Error()), err
		}

		// 更新撤销记录（其中存储包括撤销投票和撤销提名）
		tempRevokeByte := rwSet.RSet[1].PureData.GetValue() // 获取最新数据
		tempRevokeValue := NewRevokeValue()
		if tempRevokeByte != nil { // 第一次撤销（提名或投票时byte一定为空）
			if err := json.Unmarshal(tempRevokeByte, &tempRevokeValue); err != nil {
				//fmt.Println("V_unmarshal error", err.Error())
				tp.log.Error("V__撤销提名-撤销记录表反序列化失败", err.Error(), "bytes", tempRevokeByte)
			}
		}
		//else{
		//	fmt.Println("first revoke(nominate or ballot) reads byte nil")
		//}
		// 撤销记录中没有对应撤销信息
		if _, ok := tempRevokeValue[contractCtx.Initiator()]; !ok {
			tempRevokeValue[contractCtx.Initiator()] = make([]revokeItem, 0)
		}
		// append
		tempRevokeValue[contractCtx.Initiator()] = append(tempRevokeValue[contractCtx.Initiator()], revokeItem{
			RevokeType:    NOMINATETYPE,
			Ballot:        ballot,
			TargetAddress: candidateName,
		})
		// 写回
		tempRevokeBytes, err := json.Marshal(tempRevokeValue)
		if err != nil {
			return common.NewContractErrResponse(common.StatusErr, err.Error()), err
		}
		if err := contractCtx.Put(tp.election.bindContractBucket, []byte(rKey), tempRevokeBytes); err != nil {
			return common.NewContractErrResponse(common.StatusErr, err.Error()), err
		}
	}
	delta := contract.Limits{
		XFee: fee,
	}
	contractCtx.AddResourceUsed(delta)
	return common.NewContractOKResponse([]byte("ok")), nil
}

// runVote 执行投票
// Args: candidate::候选人钱包地址
//       amount::投票者票数
func (tp *tdposConsensus) runVote(contractCtx contract.KContext) (*contract.Response, error) {
	// 1.1 验证合约参数是否正确
	candidateName, height, err := tp.checkArgs(contractCtx.Args())
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	amountBytes := contractCtx.Args()["amount"]
	amountStr := string(amountBytes)
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if amount <= 0 || err != nil {
		return common.NewContractErrResponse(common.StatusErr, amountErr.Error()), amountErr
	}
	// 1.2 调用冻结接口
	tokenArgs := map[string][]byte{
		"from":      []byte(contractCtx.Initiator()),
		"amount":    []byte(fmt.Sprintf("%d", amount)),
		"lock_type": []byte(utils.GovernTokenTypeTDPOS),
	}
	_, err = contractCtx.Call("xkernel", utils.GovernTokenKernelContract, "Lock", tokenArgs)
	if err != nil {
		// err1: lock gov tokens failed, query account balance error——对应完全没有购买过治理代币
		// err2: lock gov tokens failed, account available balance insufficient——对应买过治理代币，但本次投票量>剩余可用代币
		return common.NewContractErrResponse(common.StatusErr, err.Error()), errors.New("投票失败，治理代币余额不足")
	}
	// 1.3 检查vote的地址是否在候选人池中，快照读取候选人池，vote相关参数一定是会在nominate列表中显示
	res, err := tp.election.getSnapshotKey(height, tp.election.bindContractBucket, []byte(fmt.Sprintf("%s_%d_%s", tp.status.Name, tp.status.Version, nominateKey)))
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, "Internal error."), err
	}
	nominateValue := NewNominateValue()
	if err := json.Unmarshal(res, &nominateValue); err != nil {
		tp.log.Error("tdpos::runVote::load nominates read set err.")
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	if _, ok := nominateValue[candidateName]; !ok {
		return common.NewContractErrResponse(common.StatusErr, voteNominateErr.Error()), voteNominateErr
	}

	// 2. 读取投票存储
	voteKey := fmt.Sprintf("%s_%d_%s%s", tp.status.Name, tp.status.Version, voteKeyPrefix, candidateName)
	res, err = tp.election.getSnapshotKey(height, tp.election.bindContractBucket, []byte(voteKey))
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, "tdpos::Vote::get key err."), err
	}
	voteValue := NewvoteValue()
	if res != nil {
		if err := json.Unmarshal(res, &voteValue); err != nil {
			tp.log.Error("tdpos::runVote::load vote read set err.")
			return common.NewContractErrResponse(common.StatusErr, err.Error()), err
		}
	}

	// 3. 改写vote数据
	if _, ok := voteValue[contractCtx.Initiator()]; !ok {
		voteValue[contractCtx.Initiator()] = 0
	}
	voteValue[contractCtx.Initiator()] += amount
	voteBytes, err := json.Marshal(voteValue)
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	if err := contractCtx.Put(tp.election.bindContractBucket, []byte(voteKey), voteBytes); err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	// 利用读写集更新数据
	if err := contractCtx.Flush(); err != nil {
		fmt.Println("flush error", err.Error())
	}
	rwSet := contractCtx.RWSet() // 读写集
	// [0] tdpos_0_vote_TeyyPLpp9L7QAcxHangtcHTu7HUZ6iydY; [1] balanceOf_TeyyPLpp9L7QAcxHangtcHTu7HUZ6iydY
	//fmt.Println("投票RSet len", len(rwSet.RSet), string(rwSet.RSet[0].PureData.GetKey()), string(rwSet.RSet[0].PureData.GetValue()))
	//fmt.Println("投票WSet len", len(rwSet.WSet), string(rwSet.WSet[0].GetKey()), string(rwSet.WSet[0].GetValue()))
	if rwSet.RSet[0].PureData.GetValue() != nil {
		tempByte := rwSet.RSet[0].PureData.GetValue() // 读集中的数据是最新的
		tempVoteValue := NewvoteValue()
		if err := json.Unmarshal(tempByte, &tempVoteValue); err != nil { // 记得加&
			tp.log.Warn("V__投票数据反序列化失败", err.Error(), "bytes", tempByte)
		}
		number := tempVoteValue[contractCtx.Initiator()] // 从读集中获取的最新票数
		tempVoteValue[contractCtx.Initiator()] = number + amount
		//fmt.Println("number", number, "amount", amount)
		newVoteBytes, err := json.Marshal(tempVoteValue)
		if err != nil {
			return common.NewContractErrResponse(common.StatusErr, err.Error()), err
		}
		// 更新
		if err := contractCtx.Put(tp.election.bindContractBucket, []byte(voteKey), newVoteBytes); err != nil {
			return common.NewContractErrResponse(common.StatusErr, err.Error()), err
		}
	}

	delta := contract.Limits{
		XFee: fee,
	}
	contractCtx.AddResourceUsed(delta)
	return common.NewContractOKResponse([]byte("ok")), nil
}

// runRevokeVote 执行选票撤销
// 重构后的候选人撤销
// Args: candidate::候选人钱包地址
//       amount: 投票数
func (tp *tdposConsensus) runRevokeVote(contractCtx contract.KContext) (*contract.Response, error) {
	// 1.1 验证合约参数
	candidateName, height, err := tp.checkArgs(contractCtx.Args())
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	amountBytes := contractCtx.Args()["amount"]
	amountStr := string(amountBytes)
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if amount <= 0 || err != nil {
		return common.NewContractErrResponse(common.StatusErr, amountErr.Error()), amountErr
	}
	// 1.2 调用解冻接口，Args: FromAddr, amount
	tokenArgs := map[string][]byte{
		"from":      []byte(contractCtx.Initiator()),
		"amount":    []byte(fmt.Sprintf("%d", amount)),
		"lock_type": []byte(utils.GovernTokenTypeTDPOS),
	}
	_, err = contractCtx.Call("xkernel", utils.GovernTokenKernelContract, "UnLock", tokenArgs)
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	// 1.3 检查是否在vote池子里面，读取vote存储
	voteKey := fmt.Sprintf("%s_%d_%s%s", tp.status.Name, tp.status.Version, voteKeyPrefix, candidateName)
	res, err := tp.election.getSnapshotKey(height, tp.election.bindContractBucket, []byte(voteKey))
	if err != nil {
		tp.log.Error("tdpos::runRevokeVote::load vote read set err when get key.")
		return common.NewContractErrResponse(common.StatusErr, "Internal error."), err
	}
	voteValue := NewvoteValue()
	if err := json.Unmarshal(res, &voteValue); err != nil {
		tp.log.Error("tdpos::runRevokeVote::load vote read set err.")
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	v, ok := voteValue[contractCtx.Initiator()]
	if !ok {
		return common.NewContractErrResponse(common.StatusErr, notFoundErr.Error()), notFoundErr
	}
	if v < amount {
		return common.NewContractErrResponse(common.StatusErr, "Your vote amount is less than have."), emptyNominateKey
	}

	// 2. 读取撤销记录，后续改写用
	rKey := fmt.Sprintf("%s_%d_%s", tp.status.Name, tp.status.Version, revokeKey)
	res, err = tp.election.getSnapshotKey(height, tp.election.bindContractBucket, []byte(rKey))
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, "Internal error."), err
	}
	revokeValue := NewRevokeValue()
	if res != nil {
		if err := json.Unmarshal(res, &revokeValue); err != nil {
			tp.log.Error("tdpos::runRevokeCandidate::load revoke read set err.")
			return common.NewContractErrResponse(common.StatusErr, notFoundErr.Error()), err
		}
	}

	// 3. 改写撤销存储，撤销表中新增操作
	if _, ok := revokeValue[contractCtx.Initiator()]; !ok {
		revokeValue[contractCtx.Initiator()] = make([]revokeItem, 0)
	}
	revokeValue[contractCtx.Initiator()] = append(revokeValue[contractCtx.Initiator()], revokeItem{
		RevokeType:    VOTETYPE,
		Ballot:        amount,
		TargetAddress: candidateName,
	})
	revokeBytes, err := json.Marshal(revokeValue)
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}

	// 4. 改写vote数据，注意，vote即使变成null也并不影响其在候选人池中，无需重写候选人池
	voteValue[contractCtx.Initiator()] -= amount
	voteBytes, err := json.Marshal(voteValue)
	if err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	if err := contractCtx.Put(tp.election.bindContractBucket, []byte(rKey), revokeBytes); err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	if err := contractCtx.Put(tp.election.bindContractBucket, []byte(voteKey), voteBytes); err != nil {
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	// 利用读写集更新数据
	if err := contractCtx.Flush(); err != nil {
		fmt.Println("flush error", err.Error())
	}
	rwSet := contractCtx.RWSet() // 读写集
	// [0] tdpos_0_revoke; [1] tdpos_0_vote_TeyyPLpp9L7QAcxHangtcHTu7HUZ6iydY; [2] balance
	//fmt.Println("RSet revoke", string(rwSet.RSet[0].PureData.GetKey()), string(rwSet.RSet[0].PureData.GetValue()))
	//fmt.Println("WSet revoke", string(rwSet.WSet[0].GetKey()), string(rwSet.WSet[0].GetValue()))
	//fmt.Println("RSet vote", string(rwSet.RSet[1].PureData.GetKey()), string(rwSet.RSet[1].PureData.GetValue()))
	//fmt.Println("WSet vote", string(rwSet.WSet[1].GetKey()), string(rwSet.WSet[1].GetValue()))
	if rwSet.RSet[0].PureData.GetValue() != nil || rwSet.RSet[1].PureData.GetValue() != nil {
		// [1]是vote部分，更新
		tempByte := rwSet.RSet[1].PureData.GetValue() // 读集中的数据是最新的
		tempVoteValue := NewvoteValue()
		if err := json.Unmarshal(tempByte, &tempVoteValue); err != nil { // 加"&"!!!
			tp.log.Warn("V__revoke_vote_unmarshal error", err.Error(), "bytes", tempByte)
		}
		number := tempVoteValue[contractCtx.Initiator()] // 从读集中获取的最新票数
		tempVoteValue[contractCtx.Initiator()] = number - amount
		if number - amount < 0 {
			return common.NewContractErrResponse(common.StatusErr, "撤销的票数不能低于对目标所投的票数"), errors.New("撤销的票数不能低于对目标所投的票数")
		}
		newVoteBytes, err := json.Marshal(tempVoteValue)
		if err != nil {
			return common.NewContractErrResponse(common.StatusErr, err.Error()), err
		}
		// 更新
		if err := contractCtx.Put(tp.election.bindContractBucket, []byte(voteKey), newVoteBytes); err != nil {
			return common.NewContractErrResponse(common.StatusErr, err.Error()), err
		}

		// [0]是revoke部分
		tempRevokeByte := rwSet.RSet[0].PureData.GetValue()
		tempRevokeValue := NewRevokeValue()
		if err := json.Unmarshal(tempRevokeByte, &tempRevokeValue); err != nil { // 。。。又是&
			tp.log.Warn("V__revoke_unmarshal error", err.Error(), "bytes", tempRevokeByte)
		}
		if _, ok := tempRevokeValue[contractCtx.Initiator()]; !ok {
			tempRevokeValue[contractCtx.Initiator()] = make([]revokeItem, 0)
		}
		// append
		tempRevokeValue[contractCtx.Initiator()] = append(tempRevokeValue[contractCtx.Initiator()], revokeItem{
			RevokeType:    VOTETYPE,
			Ballot:        amount,
			TargetAddress: candidateName,
		})
		newRevokeBytes, err := json.Marshal(tempRevokeValue)
		if err != nil {
			return common.NewContractErrResponse(common.StatusErr, err.Error()), err
		}
		// 更新
		if err := contractCtx.Put(tp.election.bindContractBucket, []byte(rKey), newRevokeBytes); err != nil {
			return common.NewContractErrResponse(common.StatusErr, err.Error()), err
		}
	}
	delta := contract.Limits{
		XFee: fee,
	}
	contractCtx.AddResourceUsed(delta)
	return common.NewContractOKResponse([]byte("ok")), nil
}

func (tp *tdposConsensus) checkArgs(txArgs map[string][]byte) (string, int64, error) {
	candidateBytes := txArgs["candidate"]
	candidateName := string(candidateBytes)
	if candidateName == "" {
		return "", 0, nominateAddrErr
	}
	heightBytes := txArgs["height"]
	heightStr := string(heightBytes)
	height, err := strconv.ParseInt(heightStr, 10, 64)
	if err != nil {
		return "", 0, notFoundErr
	}
	if height <= tp.status.StartHeight || height > tp.election.ledger.GetTipBlock().GetHeight() {
		return "", 0, errors.New("Input height invalid. Pls wait seconds.")
	}
	return candidateName, height, nil
}

//检查分红比
func (tp *tdposConsensus) checkRatio(txArgs map[string][]byte)(int64,error){
	ratioArg := txArgs["ratio"]
	ratioName := string(ratioArg)
	ratio ,error := strconv.ParseInt(ratioName,10,64)
	if error != nil {
		return 0,error
	}
	if ratio<0 || ratio>100{
		return 0, errors.New("D__分红比例必须在0-100之间")
	}
	return ratio , nil
}

type nominateValue map[string]map[string]int64

func NewNominateValue() nominateValue {
	return make(map[string]map[string]int64)
}

type voteValue map[string]int64

func NewvoteValue() voteValue {
	return make(map[string]int64)
}

type revokeValue map[string][]revokeItem

type revokeItem struct {
	RevokeType    string
	Ballot        int64
	TargetAddress string
}

func NewRevokeValue() revokeValue {
	return make(map[string][]revokeItem)
}

func (tp *tdposConsensus) isAuthAddress(candidate string, initiator string, authRequire []string) bool {
	if strings.HasSuffix(initiator, candidate) {
		return true
	}
	for _, value := range authRequire {
		if strings.HasSuffix(value, candidate) {
			return true
		}
	}
	return false
}
