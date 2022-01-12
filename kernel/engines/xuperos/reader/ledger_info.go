package reader

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"github.com/golang/protobuf/proto"
	"github.com/superconsensus/matrixcore/bcs/ledger/xledger/ledger"
	lpb "github.com/superconsensus/matrixcore/bcs/ledger/xledger/xldgpb"
	xctx "github.com/superconsensus/matrixcore/kernel/common/xcontext"
	"github.com/superconsensus/matrixcore/kernel/engines/xuperos/common"
	"github.com/superconsensus/matrixcore/kernel/engines/xuperos/xpb"
	"github.com/superconsensus/matrixcore/lib/logs"
	"github.com/superconsensus/matrixcore/lib/utils"
	"github.com/superconsensus/matrixcore/protos"
)

type ValidatorsInfo struct {
	Validators   []string `json:"validators"`
	Miner        string   `json:"miner"`
	Curterm      int64    `json:"curterm"`
	ContractInfo string   `json:"contract"`
	BlockNum     int64
	Period       int64
}

type LedgerReader interface {
	// 查询交易信息（QueryTx）
	QueryTx(txId []byte) (*xpb.TxInfo, error)
	// 通过交易哈希（string）查询交易信息
	QueryTxString(txid string) (*xpb.TxInfo, error)
	// 查询区块ID信息（GetBlock）
	QueryBlock(blkId []byte, needContent bool) (*xpb.BlockInfo, error)
	QueryBlockHeader(blkId []byte) (*xpb.BlockInfo, error)
	// 通过区块高度查询区块信息（GetBlockByHeight）
	QueryBlockByHeight(height int64, needContent bool) (*xpb.BlockInfo, error)
	QueryBlockHeaderByHeight(height int64) (*xpb.BlockInfo, error)
	// VotesUsage 票数（治理代币）使用情况，包括投票与被投票谁，提名信息，总票数与剩余可用票数等
	VotesUsage(address string) (*protos.CandidateRatio, error)
	//
	PledgeVotingRecords(address string) (*protos.PledgeVotingResponse, error)
	//
	GetVerification(address string) (*protos.VerificationTable, error)
	// 查询tdpos投票分红接口
	GovernTokenBonusQuery(account string) (*protos.BonusQueryReply, error)

	GetSystemStatusExplorer() (*protos.BCStatusExplorer, error)
}

type ledgerReader struct {
	chainCtx *common.ChainCtx
	baseCtx  xctx.XContext
	log      logs.Logger
}

var (
	// leveldb: not found
	DBNotFound = errors.New("数据库表没有记录")
	// 其它错误
	ParseErr = errors.New("解析数据库记录错误")
)

func NewLedgerReader(chainCtx *common.ChainCtx, baseCtx xctx.XContext) LedgerReader {
	if chainCtx == nil || baseCtx == nil {
		return nil
	}

	reader := &ledgerReader{
		chainCtx: chainCtx,
		baseCtx:  baseCtx,
		log:      baseCtx.GetLog(),
	}

	return reader
}

func (t *ledgerReader) QueryTx(txId []byte) (*xpb.TxInfo, error) {
	out := &xpb.TxInfo{}
	tx, err := t.chainCtx.Ledger.QueryTransaction(txId)
	if err != nil {
		t.log.Warn("ledger query tx error", "txId", utils.F(txId), "error", err)
		out.Status = lpb.TransactionStatus_TX_NOEXIST
		if err == ledger.ErrTxNotFound {
			// 查询unconfirmed表
			tx, _, err = t.chainCtx.State.QueryTx(txId)
			if err != nil {
				t.log.Warn("state query tx error", "txId", utils.F(txId), "error", err)
				return nil, common.ErrTxNotExist
			}
			t.log.Debug("state query tx succeeded", "txId", utils.F(txId))
			out.Status = lpb.TransactionStatus_TX_UNCONFIRM
			out.Tx = tx
			return out, nil
		}

		return nil, common.ErrTxNotExist
	}

	// 查询block状态，是否被分叉
	block, err := t.chainCtx.Ledger.QueryBlockHeader(tx.Blockid)
	if err != nil {
		t.log.Warn("query block error", "txId", utils.F(txId), "blockId", utils.F(tx.Blockid), "error", err)
		return nil, common.ErrBlockNotExist
	}

	t.log.Debug("query block succeeded", "txId", utils.F(txId), "blockId", utils.F(tx.Blockid))
	meta := t.chainCtx.Ledger.GetMeta()
	out.Tx = tx
	if block.InTrunk {
		out.Distance = meta.TrunkHeight - block.Height
		out.Status = lpb.TransactionStatus_TX_CONFIRM
	} else {
		out.Status = lpb.TransactionStatus_TX_FURCATION
	}

	return out, nil
}

// 通过交易id（string）查询交易信息
func (t *ledgerReader) QueryTxString(txid string) (*xpb.TxInfo, error) {
	txId, err := hex.DecodeString(txid)
	if err != nil {
		t.log.Warn("V__RPC查询交易哈希格式转换错误", "err", err)
		return nil, err
	}
	return t.QueryTx(txId)
}

// 注意不需要交易内容的时候不要查询
func (t *ledgerReader) QueryBlock(blkId []byte, needContent bool) (*xpb.BlockInfo, error) {
	out := &xpb.BlockInfo{}
	block, err := t.chainCtx.Ledger.QueryBlock(blkId)
	if err != nil {
		if err == ledger.ErrBlockNotExist {
			out.Status = lpb.BlockStatus_BLOCK_NOEXIST
			return out, common.ErrBlockNotExist
		}

		t.log.Warn("query block error", "err", err)
		return nil, common.ErrBlockNotExist
	}

	if needContent {
		out.Block = block
	}

	if block.InTrunk {
		out.Status = lpb.BlockStatus_BLOCK_TRUNK
	} else {
		out.Status = lpb.BlockStatus_BLOCK_BRANCH
	}

	return out, nil
}

func (t *ledgerReader) QueryBlockHeader(blkId []byte) (*xpb.BlockInfo, error) {
	out := &xpb.BlockInfo{}
	block, err := t.chainCtx.Ledger.QueryBlockHeader(blkId)
	if err != nil {
		if err == ledger.ErrBlockNotExist {
			out.Status = lpb.BlockStatus_BLOCK_NOEXIST
			return out, common.ErrBlockNotExist
		}

		t.log.Warn("query block error", "err", err)
		return nil, common.ErrBlockNotExist
	}

	out.Block = block
	if block.InTrunk {
		out.Status = lpb.BlockStatus_BLOCK_TRUNK
	} else {
		out.Status = lpb.BlockStatus_BLOCK_BRANCH
	}

	return out, nil
}

// 注意不需要交易内容的时候不要查询
func (t *ledgerReader) QueryBlockByHeight(height int64, needContent bool) (*xpb.BlockInfo, error) {
	out := &xpb.BlockInfo{}
	block, err := t.chainCtx.Ledger.QueryBlockByHeight(height)
	if err != nil {
		if err == ledger.ErrBlockNotExist {
			out.Status = lpb.BlockStatus_BLOCK_NOEXIST
			return out, nil
		}

		t.log.Warn("query block by height error", "err", err)
		return nil, common.ErrBlockNotExist
	}

	if needContent {
		out.Block = block
	}

	if block.InTrunk {
		out.Status = lpb.BlockStatus_BLOCK_TRUNK
	} else {
		out.Status = lpb.BlockStatus_BLOCK_BRANCH
	}

	return out, nil
}

// 注意不需要交易内容的时候不要查询
func (t *ledgerReader) QueryBlockHeaderByHeight(height int64) (*xpb.BlockInfo, error) {
	out := &xpb.BlockInfo{}
	block, err := t.chainCtx.Ledger.QueryBlockHeaderByHeight(height)
	if err != nil {
		if err == ledger.ErrBlockNotExist {
			out.Status = lpb.BlockStatus_BLOCK_NOEXIST
			return out, nil
		}

		t.log.Warn("query block by height error", "err", err)
		return nil, common.ErrBlockNotExist
	}

	out.Block = block
	if block.InTrunk {
		out.Status = lpb.BlockStatus_BLOCK_TRUNK
	} else {
		out.Status = lpb.BlockStatus_BLOCK_BRANCH
	}

	return out, nil
}

func (t *ledgerReader) VotesUsage(address string) (*protos.CandidateRatio, error) {

	out := &protos.CandidateRatio{}
	keytable := "ballot_" + address
	PbTxBuf, kvErr := t.chainCtx.Ledger.ConfirmedTable.Get([]byte(keytable))
	if kvErr != nil {
		return nil, DBNotFound
	}
	parserErr := proto.Unmarshal(PbTxBuf, out)
	if parserErr != nil {
		return nil, ParseErr
	}

	return out, nil
}
func (t *ledgerReader) PledgeVotingRecords(address string) (*protos.PledgeVotingResponse, error) {

	out := &protos.PledgeVotingResponse{}

	CandidateRatio := &protos.CandidateRatio{}
	VoteDetails := []*protos.VoteDetailsStatus{}
	_, error := t.ReadUserBallot(address, CandidateRatio)
	if error != nil {
		return nil, DBNotFound
	}
	out.TotalAmount = CandidateRatio.TatalVote
	// 提名时质押的金额，普通投票用户如果没有提名过是没有NominateDetails字段的，需要注意空指针
	if CandidateRatio.NominateDetails != nil {
		nominateDetails, ok := CandidateRatio.NominateDetails[address]
		if ok {
			out.Freezetotal = nominateDetails.Amount
		}
	}
	out.UsedAmount = CandidateRatio.Used
	//获取冻结信息表
	FrozenAssetsTable := &protos.FrozenAssetsTable{}
	keytable := "amount_" + address
	PbTxBuf, kvErr := t.chainCtx.Ledger.ConfirmedTable.Get([]byte(keytable))
	if kvErr != nil {
		return nil, DBNotFound
	}
	parserErr := proto.Unmarshal(PbTxBuf, FrozenAssetsTable)
	if parserErr != nil {
		return nil, ParseErr
	}
	out.FrozenAssetsTable = FrozenAssetsTable

	//获取冻结中的，也就是申请解冻
	var thawAmount int64
	for _, data := range FrozenAssetsTable.ThawDetail {
		value, err := strconv.ParseInt(data.Amount, 10, 64)
		if err == nil {
			thawAmount += value
		}
	}
	freeString := strconv.FormatInt(thawAmount, 10)
	out.FreezeAmount = freeString

	//获取投票信息
	for key, data := range CandidateRatio.MyVoting {
		VoteDetailsStatus := &protos.VoteDetailsStatus{}
		//获取奖励比
		userBallot := &protos.CandidateRatio{}
		_, err := t.ReadUserBallot(key, userBallot)
		if err != nil {
			return out, DBNotFound
		}
		VoteDetailsStatus.Ratio = int32(userBallot.Ratio)
		VoteDetailsStatus.Toaddr = key
		VoteDetailsStatus.Ballots, _ = strconv.ParseInt(data, 10, 64)
		VoteDetailsStatus.Totalballots = userBallot.BeVotedTotal
		//投票为0就不显示了
		if VoteDetailsStatus.Ballots != 0 {
			VoteDetails = append(VoteDetails, VoteDetailsStatus)
		}

	}
	out.VoteDetailsStatus = VoteDetails
	out.MyVote = int64(len(VoteDetails))

	return out, nil
}

func (t *ledgerReader) ReadUserBallot(operator string, table *protos.CandidateRatio) (*protos.CandidateRatio, error) {
	keytable := "ballot_" + operator
	PbTxBuf, kvErr := t.chainCtx.Ledger.ConfirmedTable.Get([]byte(keytable))
	if kvErr != nil {
		return nil, DBNotFound
	}
	parserErr := proto.Unmarshal(PbTxBuf, table)
	if parserErr != nil {
		return nil, ParseErr
	}
	return table, nil
}

func (t *ledgerReader) GetSystemStatusExplorer() (*protos.BCStatusExplorer, error) {
	out := &protos.BCStatusExplorer{}
	//获取全网总知产
	out.TotalMoney = t.chainCtx.State.GetMeta().UtxoTotal
	//获取当前高度
	out.Height = t.chainCtx.Ledger.GetMeta().TrunkHeight

	//从所有候选人里面读
	toTable := "tdpos_freezes_total_assets"
	freetable := &protos.AllCandidate{}
	PbTxBuf, kvErr := t.chainCtx.Ledger.ConfirmedTable.Get([]byte(toTable))
	if kvErr == nil {
		parserErr := proto.Unmarshal(PbTxBuf, freetable)
		if parserErr != nil {
			t.log.Warn("D__解析tdpos_freezes_total_assets失败", "err", parserErr)
			return out, ParseErr
		}
	} else {
		t.log.Warn("D__解析tdpos_freezes_total_assets为空")
		return out, DBNotFound
	}

	//获取冻结总资产
	out.FreeMonry = freetable.Freemonry
	//计算冻结百分比
	//fmt.Printf("D__打印FreeMonry %s \n",out.FreeMonry)
	if out.FreeMonry == "" {
		return out, nil
	}
	free, _ := new(big.Int).SetString(out.FreeMonry, 10)
	total, _ := new(big.Int).SetString(out.TotalMoney, 10)
	// 精度转换
	total.Div(total, big.NewInt(100000000))
	free.Mul(free, big.NewInt(10000))
	ratio := free.Div(free, total).Int64()
	out.Percentage = fmt.Sprintf("%.2f", float64(ratio)/100)
	out.Percentage += "%"

	return out, nil
}

func (t *ledgerReader) GetVerification(address string) (*protos.VerificationTable, error) {
	//fmt.Printf("D__进入GetVerification\n")
	out := &protos.VerificationTable{}

	//获取当前出块的人
	LedgerMeta := t.chainCtx.Ledger.GetMeta()
	tipBlockId := LedgerMeta.TipBlockid
	Block, err := t.chainCtx.Ledger.QueryBlock(tipBlockId)
	if err != nil {
		t.log.Warn("query block error", "err", err, "blockId", tipBlockId)
		return nil, common.ErrBlockNotExist
	}
	curBlockNum := Block.CurBlockNum
	proposer := string(Block.Proposer)
	cycle := 0

	//获取tdpos出块验证人
	cons, error := t.chainCtx.Consensus.GetConsensusStatus()
	if error != nil {
		return nil, error
	}
	validators_info := cons.GetCurrentValidatorsInfo()
	ValidatorsInfo := &ValidatorsInfo{}
	jsErr := json.Unmarshal(validators_info, ValidatorsInfo)
	if jsErr != nil {
		return nil, jsErr
	}
	//fmt.Printf("D__打印ValidatorsInfo： %v \n",ValidatorsInfo)
	//先读取当前一轮验证人的信息
	//fmt.Println("共识中的所有验证人", ValidatorsInfo.Validators)
	for _, data := range ValidatorsInfo.Validators {
		//fmt.Printf("D__打印当前验证人: %s \n",data)
		CandidateRatio := &protos.CandidateRatio{}
		VerificationInfo := &protos.VerificationInfo{}
		// 重点：从candidateRatio表读data（验证人）
		_, error := t.ReadUserBallot(data, CandidateRatio)
		// CandidateRatio tatal_vote:"20000"  0 创世验证人（2w是因为节点1buy了否则输出为空）
		//fmt.Println("CandidateRatio", CandidateRatio, CandidateRatio.Ratio)
		//fmt.Println("example",fmt.Sprintf("%v", CandidateRatio) == "") // 购买代币之后就保持false
		//fmt.Println("ratio",fmt.Sprintf("%v", CandidateRatio.Ratio) == "0")// 提名后false，撤销true
		//fmt.Println("aa", CandidateRatio.Ratio == 0, CandidateRatio.Ratio)// 同上
		//fmt.Println("error", error)
		if error == nil && CandidateRatio.Ratio != 0 {
			// tatal_vote:"20000"
			//Ratio:10
			//is_Nominate:true
			//used:"15000"
			// nominate_details:<key:"TeyyPLpp9L7QAcxHangtcHTu7HUZ6iydY"
			// value:<amount:"15000" > >
			//fmt.Printf("D__打印当前CandidateRatio: %s\n",CandidateRatio)
			VerificationInfo.Total = CandidateRatio.BeVotedTotal
			if VerificationInfo.Total == "" {
				VerificationInfo.Total = "0"
			}
			VerificationInfo.Ratio = int32(CandidateRatio.Ratio)
			value, ok := CandidateRatio.VotingUser[address]
			if ok {
				free, _ := new(big.Int).SetString(value, 10)
				total, _ := new(big.Int).SetString(CandidateRatio.BeVotedTotal, 10)
				free.Mul(free, big.NewInt(10000))
				if total.Int64() > 0 {
					ratio := free.Div(free, total).Int64()
					VerificationInfo.Percentage = fmt.Sprintf("%.2f", float64(ratio)/100)
					VerificationInfo.Percentage += "%"
					VerificationInfo.MyTotal = value
				} else {
					VerificationInfo.Percentage = "0%"
					VerificationInfo.MyTotal = "0"
				}
			} else {
				VerificationInfo.Percentage = "0%"
				VerificationInfo.MyTotal = "0"
			}
			if out.Verification == nil {
				out.Verification = make(map[string]*protos.VerificationInfo)
			}
			out.Verification[data] = VerificationInfo
		} else if CandidateRatio.Ratio == 0 {
			// 从表中读取不到数据的为默认创世验证人
			VerificationInfo.Total = "0"
			VerificationInfo.Percentage = "0%"
			VerificationInfo.MyTotal = "0"
			// 按理来说这里应该是0，但是为0返回时该字段会被过滤，因此返回-1
			VerificationInfo.Ratio = 0
			if out.Verification == nil {
				out.Verification = make(map[string]*protos.VerificationInfo)
			}
			out.Verification[data] = VerificationInfo
			// other case，提名后被撤销
		}
	}
	out.Len = int64(len(out.Verification))

	for _, data := range ValidatorsInfo.Validators {
		if proposer != data { //判断出块到哪个验证人了
			cycle++
		} else {
			break
		}
	}

	//从所有候选人里面读
	toTable := "tdpos_freezes_total_assets"
	// allCandidate
	freetable := &protos.AllCandidate{}
	PbTxBuf, kvErr := t.chainCtx.Ledger.ConfirmedTable.Get([]byte(toTable))
	if kvErr == nil {
		parserErr := proto.Unmarshal(PbTxBuf, freetable)
		if parserErr != nil {
			t.log.Warn("D__解析tdpos_freezes_total_assets失败", "err", parserErr)
			return out, nil
		}
	} else { // 为空就没必要遍历freetable了，直接计算timeLeft
		//t.log.Warn("D__解析tdpos_freezes_total_assets为空")
		//return out,nil
		goto left
	}
	//候选人
	for _, data := range freetable.Candidate {
		CandidateRatio := &protos.CandidateRatio{}
		VerificationInfo := &protos.VerificationInfo{}
		_, error := t.ReadUserBallot(data, CandidateRatio)
		if error == nil {
			VerificationInfo.Total = CandidateRatio.BeVotedTotal
			if VerificationInfo.Total == "" {
				VerificationInfo.Total = "0"
			}
			VerificationInfo.Ratio = int32(CandidateRatio.Ratio)
			value, ok := CandidateRatio.VotingUser[address]
			if ok {
				free, _ := new(big.Int).SetString(value, 10)
				total, _ := new(big.Int).SetString(CandidateRatio.BeVotedTotal, 10)
				free.Mul(free, big.NewInt(10000))
				if total.Int64() > 0 {
					ratio := free.Div(free, total).Int64()
					VerificationInfo.Percentage = fmt.Sprintf("%.2f", float64(ratio)/100)
					VerificationInfo.Percentage += "%"
					VerificationInfo.MyTotal = value
				} else {
					VerificationInfo.Percentage = "0%"
					VerificationInfo.MyTotal = "0"
				}
			} else {
				VerificationInfo.Percentage = "0%"
				VerificationInfo.MyTotal = "0"
			}
			if out.Candidate == nil {
				out.Candidate = make(map[string]*protos.VerificationInfo)
			}
			//_ , ok = out.Verification[data]
			//if  !ok {
			out.Candidate[data] = VerificationInfo
			//}
		}
	}
	out.LenCandidate = int64(len(out.Candidate))
	//fmt.Printf("D__验证人获取完毕\n")

left:
	index := out.Len - int64(cycle)
	// 以每个矿工20个块计算（3s*20=60）
	//out.TimeLeft = index*60 - curBlockNum*3
	// 共识获取出块间隔、出块数量，出块间隔单位：毫秒
	//fmt.Println("出块数量", ValidatorsInfo.BlockNum, "出块间隔", ValidatorsInfo.Period)
	out.TimeLeft = index*ValidatorsInfo.Period*ValidatorsInfo.BlockNum/1000 - curBlockNum*ValidatorsInfo.Period/1000
	if out.TimeLeft < 0 {
		out.TimeLeft = 0
	}
	return out, nil
}

// tdpos投票分红查询RPC接口
func (t *ledgerReader) GovernTokenBonusQuery(account string) (*protos.BonusQueryReply, error) {
	l := t.chainCtx.Ledger
	bonusData := &protos.AllBonusData{}
	allBonusDataBytes, err := l.ConfirmedTable.Get([]byte("all_bonus_data"))
	if err == nil {
		pe := proto.Unmarshal(allBonusDataBytes, bonusData)
		if pe != nil {
			t.log.Warn("V__查询分红数据反序列化出错", pe)
			return nil, errors.New("V__查询分红数据反序列化出错\n")
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
		} else {
			resp = fmt.Sprintf("分红奖励总量:%v", reward)
		}

		reply := &protos.BonusQueryReply{}
		reply.Bonus = reward.Int64()
		reply.Info = resp

		// 查询最新区块，获取区块高度
		blk, err := l.QueryBlock(t.chainCtx.State.GetLatestBlockid())
		nominateTokenNeed := big.NewInt(0)
		if err != nil {
			//fmt.Println("V__获取最新区块失败")
			t.log.Warn("V__获取最新区块失败")
		} else {
			// tdpos共识提名候选人需要质押治理代币的最少量
			nominateTokenNeed = l.GetTokenRequired(blk.GetHeight())
			reply.TokenNeed = nominateTokenNeed.String()
		}
		//fmt.Println("分红总量", reward.Int64())

		return reply, nil
	} else {
		return nil, errors.New("V__目前暂无分红信息")
	}
}
