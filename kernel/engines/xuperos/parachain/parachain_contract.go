package parachain

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/superconsensus/matrixcore/bcs/ledger/xledger/ledger"
	"github.com/superconsensus/matrixcore/bcs/ledger/xledger/utils"
	"github.com/superconsensus/matrixcore/kernel/contract"
	"github.com/superconsensus/matrixcore/kernel/engines/xuperos/common"
	"github.com/superconsensus/matrixcore/protos"
	"time"
)

var (
	// ErrCreateBlockChain is returned when create block chain error
	ErrCreateBlockChain = errors.New("create blockChain error")
	ErrGroupNotFound    = errors.New("group not found")
	ErrUnAuthorized     = errors.New("权限不足，操作失败")
	ErrChainNotFound    = errors.New("该平行链不存在")
	ErrCtxEmpty         = errors.New("chain context is not found")
	ErrBcNameEmpty      = errors.New("block chain name is empty")
	ErrBcDataEmpty      = errors.New("first block data is empty")
	ErrAdminEmpty       = errors.New("no administrator")
	ErrMembersEmpty		= errors.New("成员参数members不能为空")
)

const (
	success           = 200
	unAuthorized      = 403
	targetNotFound    = 404
	internalServerErr = 500

	paraChainEventName  = "EditParaGroups"
	genesisConfigPrefix = "$G_"
	historyMsgPrefix	= "$H_"			// 历史消息表前缀
	expiredLimit		= 3600*24*3		// 单位秒。消息过期时限，默认3天
)

type paraChainContract struct {
	BcName            string
	MinNewChainAmount int64
	ChainCtx          *common.ChainCtx
}

func NewParaChainContract(bcName string, minNewChainAmount int64, chainCtx *common.ChainCtx) *paraChainContract {
	t := &paraChainContract{
		BcName:            bcName,
		MinNewChainAmount: minNewChainAmount,
		ChainCtx:          chainCtx,
	}

	return t
}

type createChainMessage struct {
	BcName        string `json:"name"`
	GenesisConfig string `json:"genesis_config"`
	Group         Group  `json:"group"`
}

type stopChainMessage struct {
	BcName string `json:"name"`
}

type refreshMessage struct {
	BcName        string `json:"name"`
	GenesisConfig string `json:"genesis_config"`
	Group         Group  `json:"group"`
}

// handleCreateChain 创建平行链的异步事件方法，若账本不存在则创建，加载链到引擎，需幂等
func (p *paraChainContract) handleCreateChain(ctx common.TaskContext) error {
	var args createChainMessage
	err := ctx.ParseArgs(&args)
	if err != nil {
		return err
	}
	// 查看当前节点是否有权限创建/获取该平行链
	haveAccess := isContain(args.Group.Admin, p.ChainCtx.Address.Address) || isContain(args.Group.Identities, p.ChainCtx.Address.Address)
	if !haveAccess {
		return nil
	}
	return p.doCreateChain(args.BcName, args.GenesisConfig)
}

func (p *paraChainContract) doCreateChain(bcName string, bcGenesisConfig string) error {
	if _, err := p.ChainCtx.EngCtx.ChainM.Get(bcName); err == nil {
		p.ChainCtx.XLog.Warn("Chain is running, no need be created", "chain", bcName)
		return nil
	}
	err := utils.CreateLedgerWithData(bcName, []byte(bcGenesisConfig), p.ChainCtx.EngCtx.EnvCfg)
	if err != nil && err != utils.ErrBlockChainExist {
		return err
	}
	if err == utils.ErrBlockChainExist {
		p.ChainCtx.XLog.Warn("Chain created before, load again", "chain", bcName)

	}
	return p.ChainCtx.EngCtx.ChainM.LoadChain(bcName)
}

// handleStopChain 关闭平行链，仅从引擎中卸载，不处理账本，需幂等
func (p *paraChainContract) handleStopChain(ctx common.TaskContext) error {
	var args stopChainMessage
	err := ctx.ParseArgs(&args)
	if err != nil {
		return err
	}
	return p.doStopChain(args.BcName)
}

func (p *paraChainContract) doStopChain(bcName string) error {
	if _, err := p.ChainCtx.EngCtx.ChainM.Get(bcName); err != nil {
		p.ChainCtx.XLog.Warn("Chain hasn't been loaded yet", "chain", bcName)
		return nil
	}
	return p.ChainCtx.EngCtx.ChainM.Stop(bcName)
}

func (p *paraChainContract) handleRefreshChain(ctx common.TaskContext) error {
	var args refreshMessage
	err := ctx.ParseArgs(&args)
	if err != nil {
		return err
	}
	// 根据当前节点目前是否有权限获取该链，决定当前是停掉链还是加载链
	haveAccess := isContain(args.Group.Admin, p.ChainCtx.Address.Address) || isContain(args.Group.Identities, p.ChainCtx.Address.Address) || isContain(args.Group.SecAdmin, p.ChainCtx.Address.Address)
	if haveAccess {
		return p.doCreateChain(args.BcName, args.GenesisConfig)
	}
	return p.doStopChain(args.BcName)
}

func (p *paraChainContract) createChain(ctx contract.KContext) (*contract.Response, error) {
	if p.BcName != p.ChainCtx.EngCtx.EngCfg.RootChain {
		return nil, ErrUnAuthorized
	}
	bcName, bcData, bcGroup, err := p.parseArgs(ctx.Args())
	if err != nil {
		return nil, err
	}

	// 1. 群组相关字段改写
	// 确保未创建过该链
	chainRes, _ := ctx.Get(ParaChainKernelContract, []byte(bcName))
	if chainRes != nil {
		return newContractErrResponse(unAuthorized, utils.ErrBlockChainExist.Error()), utils.ErrBlockChainExist
	}
	// 创建链时，自动写入Group信息
	group := &Group{
		GroupID:    bcName,
		Admin:      []string{ctx.Initiator()},
		Identities: nil,
		Invites:	nil,
		Applies:	nil,
	}
	if bcGroup != nil {
		group = bcGroup
	}
	rawBytes, err := json.Marshal(group)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	// 写群组信息
	if err := ctx.Put(ParaChainKernelContract,
		[]byte(bcName), rawBytes); err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	// 写创世块配置信息
	if err := ctx.Put(ParaChainKernelContract,
		[]byte(genesisConfigPrefix+bcName), []byte(bcData)); err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}

	// 2. 群组注册完毕后，再进行异步事件调用
	// 当该Tx被打包上链时，将运行CreateBlockChain注册的handler，并输入参数
	message := &createChainMessage{
		BcName:        bcName,
		GenesisConfig: bcData,
		Group:         *group,
	}
	err = ctx.EmitAsyncTask("CreateBlockChain", message)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}

	delta := contract.Limits{
		XFee: p.MinNewChainAmount,
	}
	ctx.AddResourceUsed(delta)

	return &contract.Response{
		Status: success,
		Body:   []byte("CreateBlockChain success"),
	}, nil
}

func (p *paraChainContract) stopChain(ctx contract.KContext) (*contract.Response, error) {
	// 1. 查看输入参数是否正确
	if p.BcName != p.ChainCtx.EngCtx.EngCfg.RootChain {
		return nil, ErrUnAuthorized
	}
	if ctx.Args()["name"] == nil {
		return nil, ErrBcNameEmpty
	}
	bcName := string(ctx.Args()["name"])
	if bcName == "" {
		return nil, ErrBcNameEmpty
	}

	// 2. 查看是否包含相关群组，确保链已经创建过
	groupBytes, err := ctx.Get(ParaChainKernelContract, []byte(bcName))
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if groupBytes == nil {
		return newContractErrResponse(unAuthorized, ErrChainNotFound.Error()), ErrChainNotFound
	}

	// 3. 查看发起者是否有权限停用
	chainGroup := Group{}
	err = json.Unmarshal(groupBytes, &chainGroup)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if !isContain(chainGroup.Admin, ctx.Initiator()) {
		return newContractErrResponse(unAuthorized, ErrUnAuthorized.Error()), ErrUnAuthorized
	}

	// 4. 将该链停掉
	message := stopChainMessage{
		BcName: bcName,
	}
	err = ctx.EmitAsyncTask("StopBlockChain", message)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}

	delta := contract.Limits{
		XFee: p.MinNewChainAmount,
	}
	ctx.AddResourceUsed(delta)

	return &contract.Response{
		Status: success,
		Body:   []byte("StopBlockChain success"),
	}, nil
}

// 邀请成员加入联盟链
func (p *paraChainContract) inviteMembers(ctx contract.KContext) (*contract.Response, error) {
	if p.BcName != p.ChainCtx.EngCtx.EngCfg.RootChain {
		return nil, ErrUnAuthorized
	}

	chainGroup, err := getChainGroupByArgs(ctx)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	args := ctx.Args()
	// todo 无效地址的判断与处理
	if args["members"] == nil {
		return newContractErrResponse(internalServerErr, ErrMembersEmpty.Error()), ErrMembersEmpty // error
	}
	// 管理员才能邀请
	if !isContain(chainGroup.Admin, ctx.Initiator()) && !isContain(chainGroup.SecAdmin, ctx.Initiator()) {
		return newContractErrResponse(unAuthorized, "管理员以上才能邀请成员加入"), errors.New("管理员以上才能邀请成员加入")
	}
	membersBytes := ctx.Args()["members"]
	tmpGroup := Group{}
	err = json.Unmarshal(membersBytes, &tmpGroup.Identities)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	nowTime := time.Now()
	historyMsg, err := getHistoryMsg(ctx, chainGroup.GroupID)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if historyMsg.Invites == nil {
		historyMsg.Invites = make([]Invite, 0)
	}
	for _, invite := range tmpGroup.Identities {
		if isContain(chainGroup.Identities, invite) {
			return newContractErrResponse(internalServerErr, invite + "已经在成员中"), errors.New(invite + "已经在成员中")
		}
		inviteValue, ok := chainGroup.Invites[invite]
		if ok {
			if nowTime.Sub(time.Unix(inviteValue, 0)).Seconds() <= expiredLimit {
				return newContractErrResponse(internalServerErr, invite + "已经邀请过，等待对方处理"), errors.New(invite + "已经邀请过，等待对方处理")
			}
			// 过期的邀请信息再覆盖
			chainGroup.Invites[invite] = nowTime.Unix()
		}
		applyValue, ok := chainGroup.Applies[invite]
		if ok {
			if nowTime.Sub(time.Unix(applyValue, 0)).Seconds() <= expiredLimit {
				return newContractErrResponse(internalServerErr, invite + "在申请表中，直接同意即可"), errors.New(invite + "在申请表中，直接同意即可")
			}
		}
		// 添加邀请
		if chainGroup.Invites == nil {
			chainGroup.Invites = make(map[string]int64)
		}
		chainGroup.Invites[invite] = nowTime.Unix()
		// 历史记录
		historyMsg.Invites = append(historyMsg.Invites, Invite{
			Inviter: ctx.Initiator(),
			Invitees: invite,
			When: nowTime.Unix(),
			Result: 01, // waiting for handling
		})
	}

	// 写回
	rawBytes, err := json.Marshal(chainGroup)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if err := ctx.Put(ParaChainKernelContract, []byte(chainGroup.GroupID), rawBytes); err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	rawBytes, err = json.Marshal(historyMsg)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if err := ctx.Put(ParaChainKernelContract, []byte(historyMsgPrefix + historyMsg.Name), rawBytes); err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}

	delta := contract.Limits{
		XFee: p.MinNewChainAmount, // 1000
	}
	ctx.AddResourceUsed(delta)
	return &contract.Response{
		Status: success,
		Body:   []byte("Invite success"),
	}, nil
}

// 处理邀请加入联盟链（可同意，可拒绝）
func (p paraChainContract) inviteHandle(ctx contract.KContext) (*contract.Response, error) {
	// 参数处理
	chainGroup, err := getChainGroupByArgs(ctx)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	agree, ok := ctx.Args()["agree"]
	if !ok && ( string(agree) == "yes" || string(agree) == "no" ) {
		return newContractErrResponse(internalServerErr, "参数agree缺失或无效"), errors.New("参数agree缺失或无效")
	}
	if isContain(chainGroup.Identities, ctx.Initiator()) {
		return newContractErrResponse(internalServerErr, "已经是联盟成员"), errors.New("已经是联盟成员")
	}

	// 处理邀请表
	value, ok := chainGroup.Invites[ctx.Initiator()]
	if !ok {
		return newContractErrResponse(internalServerErr, "不在邀请表中"), errors.New("不在邀请表中")
	}

	// 判断过期(超过三天的申请不能处理)
	if time.Now().Sub(time.Unix(value, 0)).Seconds() >= expiredLimit {
		return newContractErrResponse(internalServerErr, "邀请信息已过期，不能处理"), errors.New("邀请信息已过期，不能处理")
	}

	historyMsg, err := getHistoryMsg(ctx, chainGroup.GroupID)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	// 找出对应的申请信息
	for i, invite := range historyMsg.Invites {
		if invite.When == value && invite.Invitees == ctx.Initiator(){ // 时间戳对上且被邀请人是自己
			if string(agree) == "yes" {
				historyMsg.Invites[i].Result = 02 // 02-passed
				break
			}else{
				historyMsg.Invites[i].Result = 03 // 03-reject
				break
			}
		}
	}
	// 最新的处理消息删除
	delete(chainGroup.Invites, ctx.Initiator())
	// 同意加入 且申请未过期
	if string(agree) == "yes" {
		// 处理成员表
		chainGroup.Identities = append(chainGroup.Identities, ctx.Initiator())
	}

	// 修改记录后写回
	rawBytes, err := json.Marshal(chainGroup)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if err := ctx.Put(ParaChainKernelContract, []byte(chainGroup.GroupID), rawBytes); err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	rawBytes, err = json.Marshal(historyMsg)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if err := ctx.Put(ParaChainKernelContract, []byte(historyMsgPrefix+historyMsg.Name), rawBytes); err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}

	if string(agree) == "yes" {
		// 通知event
		e := protos.ContractEvent{
			Name: paraChainEventName,
			Body: rawBytes,
		}
		ctx.AddEvent(&e)

		// 发起另一个异步事件，旨在根据不同链的状况停掉链或者加载链
		genesisConfig, err := ctx.Get(ParaChainKernelContract, []byte(genesisConfigPrefix+chainGroup.GroupID))
		if err != nil {
			err = fmt.Errorf("get genesis config failed when edit the group, bcName = %s, err = %v", chainGroup.GroupID, err)
			return newContractErrResponse(targetNotFound, ErrGroupNotFound.Error()), err
		}
		message := &refreshMessage{
			BcName:        chainGroup.GroupID,
			GenesisConfig: string(genesisConfig),
			Group:         chainGroup,
		}
		err = ctx.EmitAsyncTask("RefreshBlockChain", message)
		if err != nil {
			return newContractErrResponse(internalServerErr, err.Error()), err
		}
	}

	delta := contract.Limits{
		XFee: p.MinNewChainAmount, // 1000
	}
	ctx.AddResourceUsed(delta)
	return &contract.Response{
		Status: success,
		Body:   []byte("handle invite success"),
	}, nil
}

// 管理员剔除成员
func (p *paraChainContract) removeMembers(ctx contract.KContext) (*contract.Response, error) {
	chainGroup, err := getChainGroupByArgs(ctx)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	args := ctx.Args()
	if args["members"] == nil {
		return newContractErrResponse(internalServerErr, ErrMembersEmpty.Error()), ErrMembersEmpty // error
	}
	// 管理员才能移除成员
	if !isContain(chainGroup.Admin, ctx.Initiator()) && !isContain(chainGroup.SecAdmin, ctx.Initiator()){
		return newContractErrResponse(unAuthorized, ErrUnAuthorized.Error()), ErrUnAuthorized
	}

	removeBytes := ctx.Args()["members"]
	tmpGroup := Group{}
	err = json.Unmarshal(removeBytes, &tmpGroup.Identities)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}

	for _, identity := range tmpGroup.Identities {
		// 普通管理员只能移除成员
		afterRm, flag := removeItem(chainGroup.Identities, identity)
		if flag {
			chainGroup.Identities = afterRm
		}else {
			return newContractErrResponse(internalServerErr, "移除失败，" + identity + "不是成员"), errors.New("移除失败，" + identity + "不是成员")
		}
	}

	if isContain(chainGroup.Admin, ctx.Initiator()) {
		// 只有创链者才能直接移除管理员
		for _, s := range tmpGroup.SecAdmin {
			afterRm, flag := removeItem(chainGroup.SecAdmin, s)
			if flag {
				chainGroup.SecAdmin = afterRm
			}else {
				return newContractErrResponse(internalServerErr, "移除失败，" + s + "不是成员"), errors.New("移除失败，" + s + "不在管理员中")
			}
		}
	}

	// 通知并刷新
	resp, err := refreshChain(chainGroup, ctx)
	if err != nil {
		return resp, err
	}

	delta := contract.Limits{
		XFee: p.MinNewChainAmount, // 1000
	}
	ctx.AddResourceUsed(delta)
	return &contract.Response{
		Status: success,
		Body:   []byte("handle success"),
	}, nil
}

func (p *paraChainContract) parseArgs(args map[string][]byte) (string, string, *Group, error) {
	// 检查bcName和bcData是否合法
	bcName := ""
	bcData := ""
	if args["name"] == nil {
		return bcName, bcData, nil, ErrBcNameEmpty
	}
	if args["data"] == nil {
		return bcName, bcData, nil, ErrBcDataEmpty
	}
	bcName = string(args["name"])
	bcData = string(args["data"])
	if bcName == "" {
		return bcName, bcData, nil, ErrBcNameEmpty
	}
	if bcName == p.ChainCtx.EngCtx.EngCfg.RootChain {
		return bcName, bcData, nil, utils.ErrBlockChainExist
	}
	if bcData == "" {
		return bcName, bcData, nil, ErrBcDataEmpty
	}
	// check data format, prevent panic
	bcCfg := &ledger.RootConfig{}
	err := json.Unmarshal(args["data"], bcCfg)
	if err != nil {
		return bcName, bcData, nil, fmt.Errorf("first block data error.err:%v", err)
	}
	if args["group"] == nil {
		return bcName, bcData, nil, nil
	}

	// 若群组存在检查群组是否合法
	var bcGroup Group
	err = json.Unmarshal(args["group"], &bcGroup)
	if err != nil {
		return bcName, bcData, nil, fmt.Errorf("group data error.err:%v", err)
	}
	if bcGroup.GroupID != bcName {
		return bcName, bcData, nil, fmt.Errorf("group name should be same with the parachain name")
	}
	if len(bcGroup.Admin) == 0 {
		return bcName, bcData, nil, ErrAdminEmpty
	}
	return bcName, bcData, &bcGroup, nil
}

//////////// Group ///////////
type Group struct {
	GroupID    string   `json:"name,omitempty"`
	Admin      []string `json:"admin,omitempty"` 		// 创链者。为兼容旧版仍使用切片
	SecAdmin   []string `json:"sec_admin,omitempty"` 	// Secondary administrator. 二级管理员，同级别之间不能互相管理
	Identities []string `json:"identities,omitempty"`	// 正式成员表
	Invites	   map[string]int64 `json:"invites,omitempty"`	// 邀请表 key:受邀人address value:邀请时间戳（对同一个邀请对象只保留最新的记录，即邀请过期后再邀请相同用户时会覆盖旧消息——每次邀请成功都会写记录到History表）
	Applies    map[string]int64 `json:"applies,omitempty"`	// 申请表 同上 handle时nowTime-map[address] >= 阈值(expiredLimit秒)时不能处理
}

// 邀请记录
type Invite struct {
	Inviter		string 	`json:"inviter,omitempty"`	// 邀请人
	Invitees	string	`json:"invitees,omitempty"`	// 被邀请人
	When		int64	`json:"when,omitempty"`		// 邀请时间戳
	Result		int64	`json:"result,omitempty"`	// 结果 01-待处理（可能过期，具体由getHistory时用when与now判断，04-过期），02-通过，03-拒绝
}

// 申请记录
type Apply struct {
	Applicant	string	`json:"applicant,omitempty"`// 申请人
	When		int64	`json:"when,omitempty"`		// 申请时间戳
	Result		int64	`json:"result,omitempty"`	// 结果 01-待处理（可能过期，具体由getHistory时用when与now判断，04-过期），02-通过，03-拒绝
}

// 关于历史邀请/申请记录的消息表（即退出链与管理员移除成员时不会更新此表）
type History struct {
	Name 	string		`json:"name,omitempty"`
	Invites []Invite	`json:"invites,omitempty"` // 邀请记录
	Applies []Apply		`json:"applies,omitempty"` // 申请记录
}

// methodEditGroup 控制平行链对应的权限管理，被称为平行链群组or群组，旨在向外提供平行链权限信息
func (p *paraChainContract) editGroup(ctx contract.KContext) (*contract.Response, error) {
	group := &Group{}
	group, err := loadGroupArgs(ctx.Args(), group)
	if err != nil {
		return newContractErrResponse(targetNotFound, err.Error()), err
	}
	// 1. 查看Group群组是否存在
	groupBytes, err := ctx.Get(ParaChainKernelContract, []byte(group.GroupID))
	if err != nil {
		return newContractErrResponse(targetNotFound, ErrGroupNotFound.Error()), err
	}

	// 2. 查看发起者是否有权限修改
	chainGroup := Group{}
	err = json.Unmarshal(groupBytes, &chainGroup)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if !isContain(chainGroup.Admin, ctx.Initiator()) {
		return newContractErrResponse(unAuthorized, ErrUnAuthorized.Error()), ErrUnAuthorized
	}

	// 3. 发起修改
	if group.Admin == nil { // 必须要有admin权限
		group.Admin = chainGroup.Admin
	}
	rawBytes, err := json.Marshal(group)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if err := ctx.Put(ParaChainKernelContract, []byte(group.GroupID), rawBytes); err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}

	// 4. 通知event
	e := protos.ContractEvent{
		Name: paraChainEventName,
		Body: rawBytes,
	}
	ctx.AddEvent(&e)

	// 5. 发起另一个异步事件，旨在根据不同链的状况停掉链或者加载链
	genesisConfig, err := ctx.Get(ParaChainKernelContract, []byte(genesisConfigPrefix+group.GroupID))
	if err != nil {
		err = fmt.Errorf("get genesis config failed when edit the group, bcName = %s, err = %v", group.GroupID, err)
		return newContractErrResponse(targetNotFound, ErrGroupNotFound.Error()), err
	}
	message := &refreshMessage{
		BcName:        group.GroupID,
		GenesisConfig: string(genesisConfig),
		Group:         *group,
	}
	err = ctx.EmitAsyncTask("RefreshBlockChain", message)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}

	delta := contract.Limits{
		XFee: p.MinNewChainAmount,
	}
	ctx.AddResourceUsed(delta)
	return &contract.Response{
		Status: success,
		Body:   []byte("Edit Group success"),
	}, nil
}

// methodGetGroup 平行链群组读方法
func (p *paraChainContract) getGroup(ctx contract.KContext) (*contract.Response, error) {
	group := &Group{}
	group, err := loadGroupArgs(ctx.Args(), group)
	if err != nil {
		return newContractErrResponse(targetNotFound, err.Error()), err
	}
	groupBytes, err := ctx.Get(ParaChainKernelContract, []byte(group.GroupID))
	if err != nil {
		return newContractErrResponse(targetNotFound, ErrGroupNotFound.Error()), err
	}
	err = json.Unmarshal(groupBytes, group)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	// 仅已在群组中的节点才可以访问该key
	if !isContain(group.Admin, ctx.Initiator()) && !isContain(group.Identities, ctx.Initiator()) && !isContain(group.SecAdmin, ctx.Initiator()) {
		// 申请者，被邀请者可以查看部分信息
		/*_, ok := group.Invites[ctx.Initiator()]
		if ok {
			// 在邀请表中，隐藏现有成员+申请信息
			group.Identities = nil
			group.Applies = nil
			groupBytes, _ = json.Marshal(group)
			goto ret
		}
		_, ok = group.Applies[ctx.Initiator()]
		if ok {
			// 在申请表中，隐藏现有成员+邀请信息
			group.Identities = nil
			group.Invites = nil
			groupBytes, _ = json.Marshal(group)
			goto ret
		}*/
		return newContractErrResponse(unAuthorized, ErrUnAuthorized.Error()), nil
	}
	//ret:
	return &contract.Response{
		Status: success,
		Body:   groupBytes,
	}, nil
}

// 申请加入
func (p *paraChainContract) joinApply(ctx contract.KContext) (*contract.Response, error) {
	chainGroup, err := getChainGroupByArgs(ctx)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if isContain(chainGroup.Identities, ctx.Initiator()) {
		return newContractErrResponse(internalServerErr, "已经是联盟成员"), errors.New("已经是联盟成员")
	}
	historyMsg, err := getHistoryMsg(ctx, chainGroup.GroupID)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	value, ok := chainGroup.Invites[ctx.Initiator()] //对于过期的邀请可以继续申请加入
	if ok {
		if time.Now().Sub(time.Unix(value, 0)).Seconds() <= expiredLimit {
			return newContractErrResponse(internalServerErr, "已经在被邀请表中，直接处理通过即可"), errors.New("已经在被邀请表中，直接处理通过即可")
		}
	}
	value, ok = chainGroup.Applies[ctx.Initiator()]
	if ok {
		if time.Now().Sub(time.Unix(value, 0)).Seconds() <= expiredLimit {
			return newContractErrResponse(internalServerErr, "已经申请过了，等待管理员审核"), errors.New("已经申请过了，等待管理员审核")
		}
	}
	// 添加申请
	if chainGroup.Applies == nil {
		chainGroup.Applies = make(map[string]int64)
	}
	nowTime := time.Now()
	chainGroup.Applies[ctx.Initiator()] = nowTime.Unix()
	if historyMsg.Applies == nil {
		historyMsg.Applies = make([]Apply, 0)
	}
	historyMsg.Applies = append(historyMsg.Applies, Apply{
		Applicant: ctx.Initiator(),
		When: nowTime.Unix(),
		Result: 01,
	})

	// 写回
	rawBytes, err := json.Marshal(chainGroup)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if err := ctx.Put(ParaChainKernelContract, []byte(chainGroup.GroupID), rawBytes); err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	rawBytes, err = json.Marshal(historyMsg)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if err := ctx.Put(ParaChainKernelContract, []byte(historyMsgPrefix + historyMsg.Name), rawBytes); err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}

	delta := contract.Limits{
		XFee: p.MinNewChainAmount, // 1000
	}
	ctx.AddResourceUsed(delta)
	return &contract.Response{
		Status: success,
		Body:   []byte("handle success"),
	}, nil
}

// 处理加入申请
func (p *paraChainContract) joinHandle(ctx contract.KContext) (*contract.Response, error) {
	// 参数解析
	chainGroup, err := getChainGroupByArgs(ctx)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if !isContain(chainGroup.Admin, ctx.Initiator()) && !isContain(chainGroup.SecAdmin, ctx.Initiator()) {
		return newContractErrResponse(internalServerErr, ErrUnAuthorized.Error()), ErrUnAuthorized
	}

	applicantsBytes := ctx.Args()["applicants"] // 申请人	["aa", "bb", "cc"]
	agreesBytes := ctx.Args()["agrees"]			// 是否同意	["no", "yes", "yes"]
	if applicantsBytes == nil {
		return newContractErrResponse(internalServerErr, "处理申请人参数applicants缺失"), errors.New("处理申请人参数applicants缺失")
	}
	if agreesBytes == nil {
		return newContractErrResponse(internalServerErr, "处理申请参数agrees缺失"), errors.New("处理申请参数agrees缺失")
	}
	tmpGroup := Group{} // 临时存放解析参数，因为invites与applies表都改为map结构，只能反序列化到identities与secAdmin中
	err = json.Unmarshal(applicantsBytes, &tmpGroup.Identities)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	// 这里tmpGroup.secAdmin只是用来存放agrees参数的解析结果
	err = json.Unmarshal(agreesBytes, &tmpGroup.SecAdmin)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if len(tmpGroup.Identities) != len(tmpGroup.SecAdmin) {
		return newContractErrResponse(internalServerErr, "输入参数applicants与agrees长度应一致"), errors.New("输入参数applicants与agrees长度应一致")
	}

	historyMsg, err := getHistoryMsg(ctx, chainGroup.GroupID)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}

	needRefresh := false // 如果同意申请节点中的任意一个加入网络的话，就需要refresh
	for i, apply := range tmpGroup.Identities {
		if isContain(chainGroup.Identities, apply) {
			return newContractErrResponse(internalServerErr, apply + "已经是联盟成员"), errors.New(apply + "已经是联盟成员")// todo sync? 多个管理员同时对一个申请处理的情况
		}
		// 处理申请表
		value, ok := chainGroup.Applies[apply]
		if !ok {
			return newContractErrResponse(internalServerErr, apply + "不在申请列表中"), errors.New(apply + "不在申请列表中")
		}

		// 过期不能处理
		if time.Now().Sub(time.Unix(value, 0)).Seconds() >= expiredLimit {
			return newContractErrResponse(internalServerErr, "部分申请已经过期，无法处理"), errors.New("部分申请已经过期，无法处理")
		}
		// 已经处理的消息删除（对于Group表）
		delete(chainGroup.Applies, apply)
		// 同意加入
		var index int // 用来记录处理的是具体哪个申请
		for i3, a := range historyMsg.Applies {
			if a.When == value && a.Applicant == apply { // 多加了一层申请人的判断
				index = i3
				break
			}
		}
		if tmpGroup.SecAdmin[i] == "yes" {
			needRefresh = true
			// 添加到成员表
			chainGroup.Identities = append(chainGroup.Identities, apply)
			historyMsg.Applies[index].Result = 02 // passed
		}else if tmpGroup.SecAdmin[i] == "no" {
			historyMsg.Applies[index].Result = 03 // reject
		}else {
			return newContractErrResponse(internalServerErr, "部分agree参数错误"), errors.New("部分agree参数错误")
		}
	}

	// 修改记录后写回
	rawBytes, err := json.Marshal(chainGroup)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if err := ctx.Put(ParaChainKernelContract, []byte(chainGroup.GroupID), rawBytes); err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	rawBytes, err = json.Marshal(historyMsg)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if err := ctx.Put(ParaChainKernelContract, []byte(historyMsgPrefix + historyMsg.Name), rawBytes); err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}

	if needRefresh {
		// 通知event
		e := protos.ContractEvent{
			Name: paraChainEventName,
			Body: rawBytes,
		}
		ctx.AddEvent(&e)

		// 发起另一个异步事件，旨在根据不同链的状况停掉链或者加载链
		genesisConfig, err := ctx.Get(ParaChainKernelContract, []byte(genesisConfigPrefix+chainGroup.GroupID))
		if err != nil {
			err = fmt.Errorf("get genesis config failed when edit the group, bcName = %s, err = %v", chainGroup.GroupID, err)
			return newContractErrResponse(targetNotFound, ErrGroupNotFound.Error()), err
		}
		message := &refreshMessage{
			BcName:        chainGroup.GroupID,
			GenesisConfig: string(genesisConfig),
			Group:         chainGroup,
		}
		err = ctx.EmitAsyncTask("RefreshBlockChain", message)
		if err != nil {
			return newContractErrResponse(internalServerErr, err.Error()), err
		}
	}

	delta := contract.Limits{
		XFee: p.MinNewChainAmount, // 1000
	}
	ctx.AddResourceUsed(delta)
	return &contract.Response{
		Status: success,
		Body:   []byte("handle apply success"),
	}, nil
}

// 主动退出链
func (p *paraChainContract) exitChain(ctx contract.KContext) (*contract.Response, error) {
	chainGroup, err := getChainGroupByArgs(ctx)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}

	// 链创建人不能退出，如果要退出调用stop，即解散
	if isContain(chainGroup.Admin, ctx.Initiator()) {
		return newContractErrResponse(internalServerErr, "创链者不能退出，如必要可以使用stopChain接口"), errors.New("创链者不能退出，如必要可以使用stopChain接口")
	}

	// 管理员可以退出
	afterRmSecAdmin, flagSecAdmin := removeItem(chainGroup.SecAdmin, ctx.Initiator())
	if flagSecAdmin {
		chainGroup.SecAdmin = afterRmSecAdmin
	}
	// 普通成员退出
	afterRm, flag := removeItem(chainGroup.Identities, ctx.Initiator())
	if flag {
		chainGroup.Identities = afterRm
	}
	if !flagSecAdmin && !flag {
		return newContractErrResponse(internalServerErr, "不是联盟成员，不需要退出"), errors.New("不是联盟成员，不需要退出")
	}

	resp, err := refreshChain(chainGroup, ctx)
	if err != nil {
		return resp, err
	}

	delta := contract.Limits{
		XFee: p.MinNewChainAmount, // 1000
	}
	ctx.AddResourceUsed(delta)
	return &contract.Response{
		Status: success,
		Body:   []byte("handle success"),
	}, nil
}

// 创链者对管理员的管理（成员晋升为管理/管理退为成员）
func (p *paraChainContract) adminManage(ctx contract.KContext) (*contract.Response, error) {
	chainGroup, err := getChainGroupByArgs(ctx)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if !isContain(chainGroup.Admin, ctx.Initiator()) {
		return newContractErrResponse(internalServerErr, "不是创链者，权限不足"), errors.New("不是创链者，权限不足")
	}
	// for ups: chainGroup.Identities -> chainGroup.SecAdmin
	// for downs: chainGroup.SecAdmin -> chainGroup.Identities
	args := ctx.Args()
	if args["ups"] == nil && args["downs"] == nil {
		return newContractErrResponse(internalServerErr, "参数ups或downs缺失"), errors.New("参数ups或downs缺失")
	}
	// 待晋升（即普通成员）
	upsGroup := Group{}
	// 待遣退（即二级管理员）
	downsGroup := Group{}
	upsBytes, ok := args["ups"]
	if ok {
		err := json.Unmarshal(upsBytes, &upsGroup.Identities)
		if err != nil {
			return newContractErrResponse(internalServerErr, err.Error()), err
		}
	}
	for _, identity := range upsGroup.Identities {
		if isContain(chainGroup.SecAdmin, identity) {
			return newContractErrResponse(internalServerErr, identity + "已经是管理员"), errors.New(identity + "已经是管理员")
		}
		// 如果没有问题的话，循环结束时upsGroup中的元素就全部加到chainGroup中的二级管理员了
		chainGroup.SecAdmin = append(chainGroup.SecAdmin, identity)
		// append之后删除旧元素
		afterRm, flag := removeItem(chainGroup.Identities, identity)
		if flag {
			chainGroup.Identities = afterRm
		}else {
			return newContractErrResponse(internalServerErr, identity + "不是联盟成员"), errors.New(identity + "不是联盟成员")
		}
	}

	downsBytes, ok := args["downs"]
	if ok {
		err := json.Unmarshal(downsBytes, &downsGroup.SecAdmin)
		if err != nil {
			return newContractErrResponse(internalServerErr, err.Error()), err
		}
	}
	for _, downsUsr := range downsGroup.SecAdmin {
		if isContain(chainGroup.Identities, downsUsr) {
			return newContractErrResponse(internalServerErr, downsUsr + "已经是联盟成员"), errors.New(downsUsr + "已经是联盟成员")
		}
		chainGroup.Identities = append(chainGroup.Identities, downsUsr)
		// 移除管理员身份
		afterRm, flag := removeItem(chainGroup.SecAdmin, downsUsr)
		if flag {
			chainGroup.SecAdmin = afterRm
		}else {
			return newContractErrResponse(internalServerErr, downsUsr + "不是管理员"), errors.New(downsUsr + "不是管理员")
		}
	}
	// 数据写回
	rawBytes, err := json.Marshal(chainGroup)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if err := ctx.Put(ParaChainKernelContract, []byte(chainGroup.GroupID), rawBytes); err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}

	delta := contract.Limits{
		XFee: p.MinNewChainAmount, // 1000
	}
	ctx.AddResourceUsed(delta)
	return &contract.Response{
		Status: success,
		Body:   []byte("handle success"),
	}, nil
}

func getChainGroupByArgs(ctx contract.KContext) (Group, error) {
	// 参数解析
	args := ctx.Args()
	bcName := args["name"]
	if bcName == nil {
		return Group{}, ErrBcNameEmpty
	}
	chainName := string(args["name"])
	chain, _ := ctx.Get(ParaChainKernelContract, []byte(chainName))
	if chain == nil {
		return Group{}, ErrChainNotFound
	}

	// 如果链存在，查询链对应成员表
	chainGroup := Group{}
	err := json.Unmarshal(chain, &chainGroup)
	if err != nil {
		return chainGroup, err
	}
	return chainGroup, nil
}

func getHistoryMsg(ctx contract.KContext, name string) (History, error) {
	historyMsg := History{
		Name: name,
	} // 历史消息记录，get不到的话说明是第一次赋值
	historyBytes, _ := ctx.Get(ParaChainKernelContract, []byte(historyMsgPrefix + historyMsg.Name))
	if historyBytes != nil {
		err := json.Unmarshal(historyBytes, &historyMsg)
		if err != nil {
			return historyMsg, err
		}
	}
	return historyMsg, nil
}

func refreshChain(chainGroup Group, ctx contract.KContext) (*contract.Response, error) {
	rawBytes, err := json.Marshal(chainGroup)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	if err := ctx.Put(ParaChainKernelContract, []byte(chainGroup.GroupID), rawBytes); err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	// 通知event
	e := protos.ContractEvent{
		Name: paraChainEventName,
		Body: rawBytes,
	}
	ctx.AddEvent(&e)

	// 发起另一个异步事件，旨在根据不同链的状况停掉链或者加载链
	genesisConfig, err := ctx.Get(ParaChainKernelContract, []byte(genesisConfigPrefix+chainGroup.GroupID))
	if err != nil {
		err = fmt.Errorf("get genesis config failed when edit the group, bcName = %s, err = %v", chainGroup.GroupID, err)
		return newContractErrResponse(targetNotFound, ErrGroupNotFound.Error()), err
	}
	message := &refreshMessage{
		BcName:        chainGroup.GroupID,
		GenesisConfig: string(genesisConfig),
		Group:         chainGroup,
	}
	err = ctx.EmitAsyncTask("RefreshBlockChain", message)
	if err != nil {
		return newContractErrResponse(internalServerErr, err.Error()), err
	}
	return nil, nil
}

// 查询某条链的邀请/申请记录
func (p *paraChainContract) getHistory(ctx contract.KContext) (*contract.Response, error) {
	name, ok := ctx.Args()["name"]
	if !ok {
		return nil, errors.New("参数name缺失")
	}
	historyMsg, err := getHistoryMsg(ctx, string(name))
	if err != nil {
		return nil, err
	}
	nowTime := time.Now()
	// 对邀请/申请表中result为1的数据进行判断，超过一定时间的视为过期，置为4再返回（但并不修改记录）
	for i, invite := range historyMsg.Invites {
		// 增加了.Result==1的判断，对已经处理的消息（2-通过/3-拒绝）直接跳过，避免了第二个条件调用time运算（算是节约了一点点点点时间）
		if invite.Result == 1 && nowTime.Sub(time.Unix(invite.When, 0)).Seconds() >= expiredLimit {
			historyMsg.Invites[i].Result = 04 // expired
		}
	}
	for i, apply := range historyMsg.Applies {
		if apply.Result == 1 && nowTime.Sub(time.Unix(apply.When, 0)).Seconds() >= expiredLimit {
			historyMsg.Applies[i].Result = 04 // expired
		}
	}
	historyByte, err := json.Marshal(historyMsg)
	if err != nil {
		return nil, err
	}
	return &contract.Response{
		Status: success,
		Body:   historyByte,
	}, nil
}

func loadGroupArgs(args map[string][]byte, group *Group) (*Group, error) {
	g := &Group{
		GroupID:    group.GroupID,
		Admin:      group.Admin,
		Identities: group.Identities,
	}
	bcNameBytes, ok := args["name"]
	if !ok {
		return nil, ErrBcNameEmpty
	}
	g.GroupID = string(bcNameBytes)
	if g.GroupID == "" {
		return nil, ErrBcNameEmpty
	}

	adminBytes, ok := args["admin"]
	if !ok {
		return g, nil
	}
	err := json.Unmarshal(adminBytes, &g.Admin)
	if err != nil {
		return nil, err
	}

	idsBytes, ok := args["identities"]
	if !ok {
		return g, nil
	}
	err = json.Unmarshal(idsBytes, &g.Identities)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func isContain(items []string, item string) bool {
	for _, eachItem := range items {
		if eachItem == item {
			return true
		}
	}
	return false
}

// 移除元素
func removeItem(items []string, item string) ([]string, bool) {
	// 标记位，item是否在items中
	flag := false
	for index, eachItem := range items {
		if eachItem == item {
			if index == len(items) - 1 { // 刚好是切片中的最后一个元素
				items = append(items[:index])
				flag = true
				break
			}
			items = append(items[:index], items[index+1:]...)
			flag = true
			break
		}
	}
	return items, flag
}

func newContractErrResponse(status int, msg string) *contract.Response {
	return &contract.Response{
		Status:  status,
		Message: msg,
	}
}
