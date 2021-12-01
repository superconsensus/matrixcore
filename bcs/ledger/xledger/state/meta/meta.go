package meta

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"

	common "github.com/superconsensus/matrixcore/kernel/consensus/base/common"
	"github.com/superconsensus/matrixcore/kernel/contract"

	"github.com/golang/protobuf/proto"
	"github.com/superconsensus/matrixcore/bcs/ledger/xledger/def"
	"github.com/superconsensus/matrixcore/bcs/ledger/xledger/ledger"
	"github.com/superconsensus/matrixcore/bcs/ledger/xledger/state/context"
	pb "github.com/superconsensus/matrixcore/bcs/ledger/xledger/xldgpb"
	"github.com/superconsensus/matrixcore/lib/logs"
	"github.com/superconsensus/matrixcore/lib/storage/kvdb"
	"github.com/superconsensus/matrixcore/protos"
)

type Meta struct {
	log       logs.Logger
	Ledger    *ledger.Ledger
	Meta      *pb.UtxoMeta  // utxo meta
	MetaTmp   *pb.UtxoMeta  // tmp utxo meta
	MutexMeta *sync.Mutex   // access control for meta
	MetaTable kvdb.Database // 元数据表，会持久化保存latestBlockid
}

var (
	ErrProposalParamsIsNegativeNumber    = errors.New("negative number for proposal parameter is not allowed")
	ErrProposalParamsIsNotPositiveNumber = errors.New("negative number of zero for proposal parameter is not allowed")
	ErrGetReservedContracts              = errors.New("Get reserved contracts error")
	// TxSizePercent max percent of txs' size in one block
	TxSizePercent = 0.8
)

// reservedArgs used to get contractnames from InvokeRPCRequest
type reservedArgs struct {
	ContractNames string
}

func NewMeta(sctx *context.StateCtx, stateDB kvdb.Database) (*Meta, error) {
	obj := &Meta{
		log:       sctx.XLog,
		Ledger:    sctx.Ledger,
		Meta:      &pb.UtxoMeta{},
		MetaTmp:   &pb.UtxoMeta{},
		MutexMeta: &sync.Mutex{},
		MetaTable: kvdb.NewTable(stateDB, pb.MetaTablePrefix),
	}

	var loadErr error
	// load consensus parameters
	obj.Meta.MaxBlockSize, loadErr = obj.LoadMaxBlockSize()
	if loadErr != nil {
		sctx.XLog.Warn("failed to load maxBlockSize from disk", "loadErr", loadErr)
		return nil, loadErr
	}
	obj.Meta.ForbiddenContract, loadErr = obj.LoadForbiddenContract()
	if loadErr != nil {
		sctx.XLog.Warn("failed to load forbiddenContract from disk", "loadErr", loadErr)
		return nil, loadErr
	}
	obj.Meta.ReservedContracts, loadErr = obj.LoadReservedContracts()
	if loadErr != nil {
		sctx.XLog.Warn("failed to load reservedContracts from disk", "loadErr", loadErr)
		return nil, loadErr
	}
	obj.Meta.NewAccountResourceAmount, loadErr = obj.LoadNewAccountResourceAmount()
	if loadErr != nil {
		sctx.XLog.Warn("failed to load newAccountResourceAmount from disk", "loadErr", loadErr)
		return nil, loadErr
	}
	// load irreversible block height & slide window parameters
	obj.Meta.IrreversibleBlockHeight, loadErr = obj.LoadIrreversibleBlockHeight()
	if loadErr != nil {
		sctx.XLog.Warn("failed to load irreversible block height from disk", "loadErr", loadErr)
		return nil, loadErr
	}
	obj.Meta.IrreversibleSlideWindow, loadErr = obj.LoadIrreversibleSlideWindow()
	if loadErr != nil {
		sctx.XLog.Warn("failed to load irreversibleSlide window from disk", "loadErr", loadErr)
		return nil, loadErr
	}
	// load gas price
	obj.Meta.GasPrice, loadErr = obj.LoadGasPrice()
	if loadErr != nil {
		sctx.XLog.Warn("failed to load gas price from disk", "loadErr", loadErr)
		return nil, loadErr
	}
	// load group chain
	obj.Meta.GroupChainContract, loadErr = obj.LoadGroupChainContract()
	if loadErr != nil {
		sctx.XLog.Warn("failed to load groupchain from disk", "loadErr", loadErr)
		return nil, loadErr
	}
	// load award
	/*obj.Meta.Award, loadErr = obj.LoadAward()
	if loadErr != nil {
		sctx.XLog.Warn("V__failed to load award from disk", "loadErr", loadErr)
		return nil, loadErr
	}*/
	// load transferFeeAmount
	obj.Meta.TransferFeeAmount, loadErr = obj.LoadTransferFeeAmount()
	if loadErr != nil {
		sctx.XLog.Warn("V__failed to load transferFeeAmount from disk", "loadErr", loadErr)
		return nil, loadErr
	}
	newMeta := proto.Clone(obj.Meta).(*pb.UtxoMeta)
	obj.MetaTmp = newMeta

	return obj, nil
}

// 执行更新
func (t *Meta) RunUpdateConfig(contractCtx contract.KContext) (*contract.Response, error) {
	//if t.Ledger == nil {
	//	return fmt.Errorf("账本未实例化，更新转账手续费失败")
	//}
	cfg, err := t.proposalArgsUnmarshal(contractCtx.Args())
	if err != nil {
		t.log.Warn("V__meta提案参数格式错误", "error", err.Error())
		return common.NewContractErrResponse(common.StatusErr, err.Error()), err
	}
	// 修改RootConfig参数
	t.Meta.TransferFeeAmount = cfg.TransferFeeAmount
	batch := t.Ledger.ConfirmBatch
	batch.Reset()
	// 更新meta中的txFee，实际手续费的校验判断都是用meta的数据
	updateErr := t.UpdateConfig(cfg, batch)
	if updateErr != nil {
		t.log.Warn("V__meta更新失败", "err", updateErr.Error())
		return common.NewContractErrResponse(common.StatusErr, updateErr.Error()), updateErr
	}
	// 记录系统配置更新
	t.log.Info("V__系统配置更新成功", "新值", cfg)
	return common.NewContractOKResponse([]byte("ok")), nil
}

// 解析提案参数
func (t *Meta) proposalArgsUnmarshal(ctxArgs map[string][]byte) (*ledger.RootConfig, error) {
	if _, ok := ctxArgs["height"]; !ok {
		t.log.Error("V__meta提案缺失生效高度参数height")
		return nil, errors.New("缺失提案生效高度参数height")
	}
	// 参数反序列化
	args := make(map[string]interface{})
	jErr := json.Unmarshal(ctxArgs["args"], &args)
	if jErr != nil {
		t.log.Error("V__配置文件反序列化失败", "err", jErr)
		return nil, jErr
	}
	// 提案文件至少需要包含下面的一个参数
	//fmt.Println("提案参数", args)
	_, okk := args["nominatePercent"]
	_, ok1 := args["txFee"]
	//_, ok2 := args["award"]
	_, ok3 := args["noFee"]
	if !(ok3 || ok1 /*|| ok2*/ || okk) {
		t.log.Error("V__meta提案缺失参数，txFee、noFee两个至少需要一个")
		return nil, errors.New("meta提案缺失参数，txFee、noFee两个至少需要一个")
	}
	value, ok := args["noFee"].(bool)
	_, ok4 := args["gasPrice"]
	if ok && !value && !ok4 { // 如果ok3-noFee-存在且值为F（F表示需要手续费），且ok4-gasPrice-缺失，报错
		t.log.Error("V__meta提案设置需要手续费模式时，gasPrice参数不能缺失")
		return nil, errors.New("V__meta提案设置需要手续费时，gasPrice参数不能缺失")
	}
	var (
		percent int64
		txfee   int64
		//award int64
		nofee    bool
		gasBytes []byte
		//gas ledger.GasPrice
		gas struct {
			CpuRate  int64 `json:"cpu_rate"`
			MemRate  int64 `json:"mem_rate"`
			DiskRate int64 `json:"disk_rate"`
			XfeeRate int64 `json:"xfee_rate"`
		}
		err error
	)
	// tdpos提名最小质押比例
	if okk { // 存在该参数则赋值percent，否则默认零值，在update时过滤0
		percent, err = strconv.ParseInt(args["nominatePercent"].(string), 10, 64)
		if err != nil {
			t.log.Warn("V__meta提案新提名最少质押比例参数类型错误", "err", err)
			return nil, err
		}
		if percent < 0 {
			t.log.Warn("V__meta提案新提名最少质押比例不能小于0")
			return nil, errors.New("V__提案新提名最少质押比例不能为负数")
		}
		//fmt.Printf("NominatePercent%#v\n", args["nominatePercent"])
	}

	// 手续费 int64，提案中应为int字符串
	if ok1 {
		txfee, err = strconv.ParseInt(args["txFee"].(string), 10, 64)
		if err != nil {
			t.log.Warn("V__meta提案修改转账手续费参数类型错误", "err", err)
			return nil, err
		}
		if txfee < 0 {
			t.log.Warn("V__meta提案手续费不能小于0")
			// 如需免手续费模式，请通过提案noFee方式修改
			return nil, errors.New("V__提案的转账手续费参数不能为负数")
		}
	}
	// 出块奖励 string，提案中应该int字符串
	/*if ok2 {
		award, err = strconv.ParseInt(args["award"].(string), 10, 64)
		if err != nil {
			t.log.Error("V__meta提案出块奖励参数类型错误", "err", err)
			return nil, err
		}
		if award == 0 {
			t.log.Error("V__出块奖励不能为0")
			return nil, errors.New("V__出块奖励不能为0")
		}
	}*/
	// 是否需要手续费模式 bool
	if ok3 { // 提案包含noFee参数
		nofee, ok = args["noFee"].(bool)
		if !ok {
			t.log.Error("V__meta提案noFee参数要求是bool型")
			return nil, errors.New("V__meta提案noFee参数要求是bool型")
		}
		// 有了前面的校验，能执行到这里自然是有gasPrice字段的
		if !nofee { // 需要手续费的情况下才修改config.gasPrice
			gasMap, _ := args["gasPrice"].(map[string]interface{})
			gasBytes, _ = json.Marshal(&gasMap)
			err := json.Unmarshal(gasBytes, &gas)
			if err != nil {
				t.log.Error("V__meta提案gasPrice参数反序列化失败", "err", err)
				return nil, errors.New("V__meta提案gasPrice参数反序列化失败")
			}
			// gas四个字段要么全0（无需手续费模式），要么全非0（需要手续费模式）；此层判断要求全非0
			if gas.MemRate == 0 || gas.CpuRate == 0 || gas.DiskRate == 0 || gas.XfeeRate == 0 {
				t.log.Error("V__meta提案需要手续费时，gasPrice四个子字段参数都不能缺失或为0")
				return nil, errors.New("V__meta提案需要手续费时，gasPrice四个子字段参数都不能缺失或为0")
			}
			// 需要手续费模式，txFee不能为0
			if txfee == 0 {
				t.log.Error("V__meta提案需要手续费模式时，txFee参数不能为0或缺失")
				return nil, errors.New("V__meta提案需要手续费模式时，txFee参数不能为0或缺失")
			}
		}
	} else {
		/* 如果没有该参数，下面RootConfig的NoFee字段默认传false，因此需要读取原先配置防止误改
		 * 举个例子，如果原先配置NoFee是true，但是提案中没有noFee字段
		 * 执行update时接收到的cfg中noFee就是false，分辨不了提案原意有没有修改
		 * award和txFee可以通过判0识别所以可以忽略
		 * 如果提案不包含noFee参数，gasPrice传默认0就行
		 */
		nofee = t.Ledger.GetNoFee()
	}
	return &ledger.RootConfig{
		// todo 其它参数待添加
		NominatePercent:   percent,
		TransferFeeAmount: txfee,
		//Award: strconv.FormatInt(award,10),
		NoFee:    nofee,
		GasPrice: gas,
	}, nil
}

// GetNewAccountResourceAmount get account for creating an account
func (t *Meta) GetNewAccountResourceAmount() int64 {
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	return t.Meta.GetNewAccountResourceAmount()
}

// LoadNewAccountResourceAmount load newAccountResourceAmount into memory
func (t *Meta) LoadNewAccountResourceAmount() (int64, error) {
	newAccountResourceAmountBuf, findErr := t.MetaTable.Get([]byte(ledger.NewAccountResourceAmountKey))
	if findErr == nil {
		utxoMeta := &pb.UtxoMeta{}
		err := proto.Unmarshal(newAccountResourceAmountBuf, utxoMeta)
		return utxoMeta.GetNewAccountResourceAmount(), err
	} else if def.NormalizedKVError(findErr) == def.ErrKVNotFound {
		genesisNewAccountResourceAmount := t.Ledger.GetNewAccountResourceAmount()
		if genesisNewAccountResourceAmount < 0 {
			return genesisNewAccountResourceAmount, ErrProposalParamsIsNegativeNumber
		}
		return genesisNewAccountResourceAmount, nil
	}

	return int64(0), findErr
}

// UpdateNewAccountResourceAmount ...
func (t *Meta) UpdateNewAccountResourceAmount(newAccountResourceAmount int64, batch kvdb.Batch) error {
	if newAccountResourceAmount < 0 {
		return ErrProposalParamsIsNegativeNumber
	}
	tmpMeta := &pb.UtxoMeta{}
	newMeta := proto.Clone(tmpMeta).(*pb.UtxoMeta)
	newMeta.NewAccountResourceAmount = newAccountResourceAmount
	newAccountResourceAmountBuf, pbErr := proto.Marshal(newMeta)
	if pbErr != nil {
		t.log.Warn("failed to marshal pb meta")
		return pbErr
	}
	err := batch.Put([]byte(pb.MetaTablePrefix+ledger.NewAccountResourceAmountKey), newAccountResourceAmountBuf)
	if err == nil {
		t.log.Info("Update newAccountResourceAmount succeed")
	}
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	t.MetaTmp.NewAccountResourceAmount = newAccountResourceAmount
	return err
}

// 从账本中加载最新的出块奖励，如果没有就获取创世区块中定义的
/*func (t *Meta) LoadAward() (string, error) {
	awardBuf, findErr := t.MetaTable.Get([]byte(ledger.AwardKey))
	if findErr == nil {
		utxoMeta := &pb.UtxoMeta{}
		err := proto.Unmarshal(awardBuf, utxoMeta)
		return utxoMeta.GetAward(), err
	} else if def.NormalizedKVError(findErr) == def.ErrKVNotFound {
		genesisAward := t.Ledger.GetAward()
		award, err := strconv.ParseInt(genesisAward, 10, 64)
		if award < 0 || err != nil {
			return genesisAward, errors.New("V__提案转账手续费非法（负数）或格式错误")
		}
		return genesisAward, nil
	}
	return "0", findErr
}*/

// 获取当前的出块奖励
/*func (t *Meta) GetAward() string {
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	return t.Meta.GetAward()
}*/

// 从账本中加载最新的手续费，如果没有就获取创世区块中定义的
func (t *Meta) LoadTransferFeeAmount() (int64, error) {
	transferFeeAmountBuf, findErr := t.MetaTable.Get([]byte(ledger.TransferFeeAmountKey))
	if findErr == nil {
		utxoMeta := &pb.UtxoMeta{}
		err := proto.Unmarshal(transferFeeAmountBuf, utxoMeta)
		return utxoMeta.GetTransferFeeAmount(), err
	} else if def.NormalizedKVError(findErr) == def.ErrKVNotFound {
		genesisTransferFeeAmount := t.Ledger.GetTransferFeeAmount()
		if genesisTransferFeeAmount < 0 {
			return genesisTransferFeeAmount, errors.New("V__提案转账手续费非法（负数）")
		}
		return genesisTransferFeeAmount, nil
	}
	return int64(0), findErr
}

// 获取当前的转账手续费
func (t *Meta) GetTransferFeeAmount() int64 {
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	return t.Meta.GetTransferFeeAmount()
}

// 更新配置，同时将其记录到账本中
func (t *Meta) UpdateConfig(cfg *ledger.RootConfig, batch kvdb.Batch) error {
	tmpMeta := &pb.UtxoMeta{}
	// tdpos提名时最少质押（占全网总资产）百分比
	if cfg.NominatePercent != 0 {
		t.Ledger.GenesisBlock.GetConfig().NominatePercent = cfg.NominatePercent
	}
	// 转账手续费
	if cfg.TransferFeeAmount != 0 {
		newMeta := proto.Clone(tmpMeta).(*pb.UtxoMeta)
		newMeta.TransferFeeAmount = cfg.TransferFeeAmount
		transferFeeAmountBuf, pbErr := proto.Marshal(newMeta)
		if pbErr != nil {
			t.log.Warn("V__update TxFee序列化meta失败")
			return pbErr
		}
		tErr := batch.Put([]byte(pb.MetaTablePrefix+ledger.TransferFeeAmountKey), transferFeeAmountBuf)
		if tErr == nil {
			t.log.Info("V__更新转账手续费transferFeeAmount成功")
		}
	}
	// 出块奖励
	/*if cfg.Award != "0" {
		awardMeta := proto.Clone(tmpMeta).(*pb.UtxoMeta)
		awardMeta.Award = cfg.Award
		awardBuf, pbErr := proto.Marshal(awardMeta)
		if pbErr != nil {
			t.log.Warn("V__update Award序列化meta失败")
			return pbErr
		}
		aErr := batch.Put([]byte(pb.MetaTablePrefix+ledger.AwardKey), awardBuf)
		if aErr == nil {
			t.log.Info("V__更新出块奖励award成功")
		}
	}*/

	t.MutexMeta.Lock()
	// 转账手续费
	if cfg.TransferFeeAmount != 0 {
		// 这里一定要注意改的是MetaTmp，直接改Meta无效
		t.MetaTmp.TransferFeeAmount = cfg.TransferFeeAmount
		// 更新ledger.genesisBlk中的txFee，最好与meta保持一致
		t.Ledger.GenesisBlock.GetConfig().TransferFeeAmount = cfg.TransferFeeAmount
	}
	// 出块奖励
	/*if cfg.Award != "0" {
		t.MetaTmp.Award = cfg.Award
		t.Ledger.GenesisBlock.GetConfig().Award = cfg.Award
	}*/
	// 无需手续费模式
	t.Ledger.GenesisBlock.GetConfig().NoFee = cfg.NoFee
	gas := &protos.GasPrice{} // 字段默认有零值
	if !cfg.NoFee {           // 提案noFee模式为F，表示需要手续费
		gas.CpuRate = cfg.GasPrice.CpuRate
		gas.MemRate = cfg.GasPrice.MemRate
		gas.DiskRate = cfg.GasPrice.DiskRate
		gas.XfeeRate = cfg.GasPrice.XfeeRate
	}
	// 前面锁过了，updateGasPrice中也会有锁，这里要先释放，避免死锁
	t.MutexMeta.Unlock()
	gErr := t.UpdateGasPrice(gas, batch)
	if gErr == nil {
		t.log.Info("V__meta提案更新gasPrice成功")
	}
	return nil
}

// GetMaxBlockSize get max block size effective in Utxo
func (t *Meta) GetMaxBlockSize() int64 {
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	return t.Meta.GetMaxBlockSize()
}

// LoadMaxBlockSize load maxBlockSize into memory
func (t *Meta) LoadMaxBlockSize() (int64, error) {
	maxBlockSizeBuf, findErr := t.MetaTable.Get([]byte(ledger.MaxBlockSizeKey))
	if findErr == nil {
		utxoMeta := &pb.UtxoMeta{}
		err := proto.Unmarshal(maxBlockSizeBuf, utxoMeta)
		return utxoMeta.GetMaxBlockSize(), err
	} else if def.NormalizedKVError(findErr) == def.ErrKVNotFound {
		genesisMaxBlockSize := t.Ledger.GetMaxBlockSize()
		if genesisMaxBlockSize <= 0 {
			return genesisMaxBlockSize, ErrProposalParamsIsNotPositiveNumber
		}
		return genesisMaxBlockSize, nil
	}

	return int64(0), findErr
}

func (t *Meta) MaxTxSizePerBlock() (int, error) {
	maxBlkSize := t.GetMaxBlockSize()
	return int(float64(maxBlkSize) * TxSizePercent), nil
}

func (t *Meta) UpdateMaxBlockSize(maxBlockSize int64, batch kvdb.Batch) error {
	if maxBlockSize <= 0 {
		return ErrProposalParamsIsNotPositiveNumber
	}
	tmpMeta := &pb.UtxoMeta{}
	newMeta := proto.Clone(tmpMeta).(*pb.UtxoMeta)
	newMeta.MaxBlockSize = maxBlockSize
	maxBlockSizeBuf, pbErr := proto.Marshal(newMeta)
	if pbErr != nil {
		t.log.Warn("failed to marshal pb meta")
		return pbErr
	}
	err := batch.Put([]byte(pb.MetaTablePrefix+ledger.MaxBlockSizeKey), maxBlockSizeBuf)
	if err == nil {
		t.log.Info("Update maxBlockSize succeed")
	}
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	t.MetaTmp.MaxBlockSize = maxBlockSize
	return err
}

func (t *Meta) GetReservedContracts() []*protos.InvokeRequest {
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	return t.Meta.ReservedContracts
}

func (t *Meta) LoadReservedContracts() ([]*protos.InvokeRequest, error) {
	reservedContractsBuf, findErr := t.MetaTable.Get([]byte(ledger.ReservedContractsKey))
	if findErr == nil {
		utxoMeta := &pb.UtxoMeta{}
		err := proto.Unmarshal(reservedContractsBuf, utxoMeta)
		return utxoMeta.GetReservedContracts(), err
	} else if def.NormalizedKVError(findErr) == def.ErrKVNotFound {
		return t.Ledger.GetReservedContracts()
	}
	return nil, findErr
}

//when to register to kernel method
func (t *Meta) UpdateReservedContracts(params []*protos.InvokeRequest, batch kvdb.Batch) error {
	if params == nil {
		return fmt.Errorf("invalid reservered contract requests")
	}
	tmpNewMeta := &pb.UtxoMeta{}
	newMeta := proto.Clone(tmpNewMeta).(*pb.UtxoMeta)
	newMeta.ReservedContracts = params
	paramsBuf, pbErr := proto.Marshal(newMeta)
	if pbErr != nil {
		t.log.Warn("failed to marshal pb meta")
		return pbErr
	}
	err := batch.Put([]byte(pb.MetaTablePrefix+ledger.ReservedContractsKey), paramsBuf)
	if err == nil {
		t.log.Info("Update reservered contract succeed")
	}
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	t.MetaTmp.ReservedContracts = params
	return err
}

func (t *Meta) GetForbiddenContract() *protos.InvokeRequest {
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	return t.Meta.GetForbiddenContract()
}

func (t *Meta) GetGroupChainContract() *protos.InvokeRequest {
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	return t.Meta.GetGroupChainContract()
}

func (t *Meta) LoadGroupChainContract() (*protos.InvokeRequest, error) {
	groupChainContractBuf, findErr := t.MetaTable.Get([]byte(ledger.GroupChainContractKey))
	if findErr == nil {
		utxoMeta := &pb.UtxoMeta{}
		err := proto.Unmarshal(groupChainContractBuf, utxoMeta)
		return utxoMeta.GetGroupChainContract(), err
	} else if def.NormalizedKVError(findErr) == def.ErrKVNotFound {
		requests, err := t.Ledger.GetGroupChainContract()
		if len(requests) > 0 {
			return requests[0], err
		}
		return nil, errors.New("unexpected error")
	}
	return nil, findErr
}

func (t *Meta) LoadForbiddenContract() (*protos.InvokeRequest, error) {
	forbiddenContractBuf, findErr := t.MetaTable.Get([]byte(ledger.ForbiddenContractKey))
	if findErr == nil {
		utxoMeta := &pb.UtxoMeta{}
		err := proto.Unmarshal(forbiddenContractBuf, utxoMeta)
		return utxoMeta.GetForbiddenContract(), err
	} else if def.NormalizedKVError(findErr) == def.ErrKVNotFound {
		requests, err := t.Ledger.GetForbiddenContract()
		if len(requests) > 0 {
			return requests[0], err
		}
		return nil, errors.New("unexpected error")
	}
	return nil, findErr
}

func (t *Meta) UpdateForbiddenContract(param *protos.InvokeRequest, batch kvdb.Batch) error {
	if param == nil {
		return fmt.Errorf("invalid forbidden contract request")
	}
	tmpNewMeta := &pb.UtxoMeta{}
	newMeta := proto.Clone(tmpNewMeta).(*pb.UtxoMeta)
	newMeta.ForbiddenContract = param
	paramBuf, pbErr := proto.Marshal(newMeta)
	if pbErr != nil {
		t.log.Warn("failed to marshal pb meta")
		return pbErr
	}
	err := batch.Put([]byte(pb.MetaTablePrefix+ledger.ForbiddenContractKey), paramBuf)
	if err == nil {
		t.log.Info("Update forbidden contract succeed")
	}
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	t.MetaTmp.ForbiddenContract = param
	return err
}

func (t *Meta) LoadIrreversibleBlockHeight() (int64, error) {
	irreversibleBlockHeightBuf, findErr := t.MetaTable.Get([]byte(ledger.IrreversibleBlockHeightKey))
	if findErr == nil {
		utxoMeta := &pb.UtxoMeta{}
		err := proto.Unmarshal(irreversibleBlockHeightBuf, utxoMeta)
		return utxoMeta.GetIrreversibleBlockHeight(), err
	} else if def.NormalizedKVError(findErr) == def.ErrKVNotFound {
		return int64(0), nil
	}
	return int64(0), findErr
}

func (t *Meta) LoadIrreversibleSlideWindow() (int64, error) {
	irreversibleSlideWindowBuf, findErr := t.MetaTable.Get([]byte(ledger.IrreversibleSlideWindowKey))
	if findErr == nil {
		utxoMeta := &pb.UtxoMeta{}
		err := proto.Unmarshal(irreversibleSlideWindowBuf, utxoMeta)
		return utxoMeta.GetIrreversibleSlideWindow(), err
	} else if def.NormalizedKVError(findErr) == def.ErrKVNotFound {
		genesisSlideWindow := t.Ledger.GetIrreversibleSlideWindow()
		// negative number is not meaningful
		if genesisSlideWindow < 0 {
			return genesisSlideWindow, ErrProposalParamsIsNegativeNumber
		}
		return genesisSlideWindow, nil
	}
	return int64(0), findErr
}

func (t *Meta) GetIrreversibleBlockHeight() int64 {
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	return t.Meta.IrreversibleBlockHeight
}

func (t *Meta) GetIrreversibleSlideWindow() int64 {
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	return t.Meta.IrreversibleSlideWindow
}

func (t *Meta) UpdateIrreversibleBlockHeight(nextIrreversibleBlockHeight int64, batch kvdb.Batch) error {
	tmpMeta := &pb.UtxoMeta{}
	newMeta := proto.Clone(tmpMeta).(*pb.UtxoMeta)
	newMeta.IrreversibleBlockHeight = nextIrreversibleBlockHeight
	irreversibleBlockHeightBuf, pbErr := proto.Marshal(newMeta)
	if pbErr != nil {
		t.log.Warn("failed to marshal pb meta")
		return pbErr
	}
	err := batch.Put([]byte(pb.MetaTablePrefix+ledger.IrreversibleBlockHeightKey), irreversibleBlockHeightBuf)
	if err != nil {
		return err
	}
	t.log.Info("Update irreversibleBlockHeight succeed")
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	t.MetaTmp.IrreversibleBlockHeight = nextIrreversibleBlockHeight
	return nil
}

func (t *Meta) UpdateNextIrreversibleBlockHeight(blockHeight int64, curIrreversibleBlockHeight int64, curIrreversibleSlideWindow int64, batch kvdb.Batch) error {
	// negative number for irreversible slide window is not allowed.
	if curIrreversibleSlideWindow < 0 {
		return ErrProposalParamsIsNegativeNumber
	}
	// slideWindow为开启,不需要更新IrreversibleBlockHeight
	if curIrreversibleSlideWindow == 0 {
		return nil
	}
	// curIrreversibleBlockHeight小于0, 不符合预期，报警
	if curIrreversibleBlockHeight < 0 {
		t.log.Warn("update irreversible block height error, should be here")
		return errors.New("curIrreversibleBlockHeight is less than 0")
	}
	nextIrreversibleBlockHeight := blockHeight - curIrreversibleSlideWindow
	// 下一个不可逆高度小于当前不可逆高度，直接返回
	// slideWindow变大或者发生区块回滚
	if nextIrreversibleBlockHeight <= curIrreversibleBlockHeight {
		return nil
	}
	// 正常升级
	// slideWindow不变或变小
	if nextIrreversibleBlockHeight > curIrreversibleBlockHeight {
		err := t.UpdateIrreversibleBlockHeight(nextIrreversibleBlockHeight, batch)
		return err
	}

	return errors.New("unexpected error")
}

func (t *Meta) UpdateNextIrreversibleBlockHeightForPrune(blockHeight int64, curIrreversibleBlockHeight int64, curIrreversibleSlideWindow int64, batch kvdb.Batch) error {
	// negative number for irreversible slide window is not allowed.
	if curIrreversibleSlideWindow < 0 {
		return ErrProposalParamsIsNegativeNumber
	}
	// slideWindow为开启,不需要更新IrreversibleBlockHeight
	if curIrreversibleSlideWindow == 0 {
		return nil
	}
	// curIrreversibleBlockHeight小于0, 不符合预期，报警
	if curIrreversibleBlockHeight < 0 {
		t.log.Warn("update irreversible block height error, should be here")
		return errors.New("curIrreversibleBlockHeight is less than 0")
	}
	nextIrreversibleBlockHeight := blockHeight - curIrreversibleSlideWindow
	if nextIrreversibleBlockHeight <= 0 {
		nextIrreversibleBlockHeight = 0
	}
	err := t.UpdateIrreversibleBlockHeight(nextIrreversibleBlockHeight, batch)
	return err
}

func (t *Meta) UpdateIrreversibleSlideWindow(nextIrreversibleSlideWindow int64, batch kvdb.Batch) error {
	if nextIrreversibleSlideWindow < 0 {
		return ErrProposalParamsIsNegativeNumber
	}
	tmpMeta := &pb.UtxoMeta{}
	newMeta := proto.Clone(tmpMeta).(*pb.UtxoMeta)
	newMeta.IrreversibleSlideWindow = nextIrreversibleSlideWindow
	irreversibleSlideWindowBuf, pbErr := proto.Marshal(newMeta)
	if pbErr != nil {
		t.log.Warn("failed to marshal pb meta")
		return pbErr
	}
	err := batch.Put([]byte(pb.MetaTablePrefix+ledger.IrreversibleSlideWindowKey), irreversibleSlideWindowBuf)
	if err != nil {
		return err
	}
	t.log.Info("Update irreversibleSlideWindow succeed")
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	t.MetaTmp.IrreversibleSlideWindow = nextIrreversibleSlideWindow
	return nil
}

// GetGasPrice get gas rate to utxo
func (t *Meta) GetGasPrice() *protos.GasPrice {
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	return t.Meta.GetGasPrice()
}

// LoadGasPrice load gas rate
func (t *Meta) LoadGasPrice() (*protos.GasPrice, error) {
	gasPriceBuf, findErr := t.MetaTable.Get([]byte(ledger.GasPriceKey))
	if findErr == nil {
		utxoMeta := &pb.UtxoMeta{}
		err := proto.Unmarshal(gasPriceBuf, utxoMeta)
		return utxoMeta.GetGasPrice(), err
	} else if def.NormalizedKVError(findErr) == def.ErrKVNotFound {
		nofee := t.Ledger.GetNoFee()
		if nofee {
			gasPrice := &protos.GasPrice{
				CpuRate:  0,
				MemRate:  0,
				DiskRate: 0,
				XfeeRate: 0,
			}
			return gasPrice, nil

		} else {
			gasPrice := t.Ledger.GetGasPrice()
			cpuRate := gasPrice.CpuRate
			memRate := gasPrice.MemRate
			diskRate := gasPrice.DiskRate
			xfeeRate := gasPrice.XfeeRate
			if cpuRate < 0 || memRate < 0 || diskRate < 0 || xfeeRate < 0 {
				return nil, ErrProposalParamsIsNegativeNumber
			}
			// To be compatible with the old version v3.3
			// If GasPrice configuration is missing or value euqals 0, support a default value
			if cpuRate == 0 && memRate == 0 && diskRate == 0 && xfeeRate == 0 {
				gasPrice = &protos.GasPrice{
					CpuRate:  1000,
					MemRate:  1000000,
					DiskRate: 1,
					XfeeRate: 1,
				}
			}
			return gasPrice, nil
		}
	}
	return nil, findErr
}

// UpdateGasPrice update gasPrice parameters
func (t *Meta) UpdateGasPrice(nextGasPrice *protos.GasPrice, batch kvdb.Batch) error {
	// check if the parameters are valid
	cpuRate := nextGasPrice.GetCpuRate()
	memRate := nextGasPrice.GetMemRate()
	diskRate := nextGasPrice.GetDiskRate()
	xfeeRate := nextGasPrice.GetXfeeRate()
	if cpuRate < 0 || memRate < 0 || diskRate < 0 || xfeeRate < 0 {
		return ErrProposalParamsIsNegativeNumber
	}
	tmpMeta := &pb.UtxoMeta{}
	newMeta := proto.Clone(tmpMeta).(*pb.UtxoMeta)
	newMeta.GasPrice = nextGasPrice
	gasPriceBuf, pbErr := proto.Marshal(newMeta)
	if pbErr != nil {
		t.log.Warn("failed to marshal pb meta")
		return pbErr
	}
	err := batch.Put([]byte(pb.MetaTablePrefix+ledger.GasPriceKey), gasPriceBuf)
	if err != nil {
		return err
	}
	t.log.Info("Update gas price succeed")
	t.MutexMeta.Lock()
	defer t.MutexMeta.Unlock()
	t.MetaTmp.GasPrice = nextGasPrice
	t.Ledger.GenesisBlock.GetConfig().GasPrice.CpuRate = nextGasPrice.CpuRate
	t.Ledger.GenesisBlock.GetConfig().GasPrice.MemRate = nextGasPrice.MemRate
	t.Ledger.GenesisBlock.GetConfig().GasPrice.DiskRate = nextGasPrice.DiskRate
	t.Ledger.GenesisBlock.GetConfig().GasPrice.XfeeRate = nextGasPrice.XfeeRate
	return nil
}
