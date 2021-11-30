package utxo

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/superconsensus-chain/xupercore/lib/storage/kvdb"
	"math/big"
	"math/rand"
	"sort"
	"strconv"
	"time"

	pb "github.com/superconsensus-chain/xupercore/bcs/ledger/xledger/xldgpb"
	"github.com/superconsensus-chain/xupercore/protos"
)

func (uv *UtxoVM) SelectUtxosBySize(fromAddr string, needLock, excludeUnconfirmed bool) ([]*protos.TxInput, [][]byte, *big.Int, error) {
	uv.log.Trace("start to merge utxos", "address", fromAddr)

	// Total amount selected
	amount := big.NewInt(0)
	maxTxSizePerBlock, _ := uv.metaHandle.MaxTxSizePerBlock()
	maxTxSize := big.NewInt(int64(maxTxSizePerBlock / 2))
	willLockKeys := make([][]byte, 0)
	txInputs := []*protos.TxInput{}
	txInputSize := int64(0)

	// same as the logic of SelectUTXO
	uv.clearExpiredLocks()

	addrPrefix := fmt.Sprintf("%s%s_", pb.UTXOTablePrefix, fromAddr)
	it := uv.ldb.NewIteratorWithPrefix([]byte(addrPrefix))
	defer it.Release()

	for it.Next() {
		key := append([]byte{}, it.Key()...)
		utxoItem := new(UtxoItem)
		// 反序列化utxoItem
		uErr := utxoItem.Loads(it.Value())
		if uErr != nil {
			uv.log.Warn("load utxo failed, skipped", "key", key)
			continue
		}
		// check if the utxo item has been frozen
		if utxoItem.FrozenHeight > uv.ledger.GetMeta().GetTrunkHeight() || utxoItem.FrozenHeight == -1 {
			uv.log.Debug("utxo still frozen, skipped", "key", key)
			continue
		}
		// lock utxo to be selected
		if needLock {
			if uv.tryLockKey(key) {
				willLockKeys = append(willLockKeys, key)
			} else {
				uv.log.Debug("can not lock the utxo key, conflict", "key", key)
				continue
			}
		} else if uv.isLocked(key) {
			// If the utxo has been locked
			uv.log.Debug("utxo locked, skipped", "key", key)
			continue
		}

		realKey := bytes.Split(key[len(pb.UTXOTablePrefix):], []byte("_"))
		refTxid, _ := hex.DecodeString(string(realKey[1]))

		if excludeUnconfirmed { //必须依赖已经上链的tx的UTXO
			isOnChain := uv.ledger.IsTxInTrunk(refTxid)
			if !isOnChain {
				if needLock {
					uv.UnlockKey(key)
				}
				continue
			}
		}
		offset, _ := strconv.Atoi(string(realKey[2]))
		// build a tx input
		txInput := &protos.TxInput{
			RefTxid:      refTxid,
			RefOffset:    int32(offset),
			FromAddr:     []byte(fromAddr),
			Amount:       utxoItem.Amount.Bytes(),
			FrozenHeight: utxoItem.FrozenHeight,
		}

		txInputs = append(txInputs, txInput)
		amount.Add(amount, utxoItem.Amount)
		txInputSize += int64(proto.Size(txInput))

		// check size
		txInputSize := big.NewInt(txInputSize)
		if txInputSize.Cmp(maxTxSize) == 1 {
			txInputs = txInputs[:len(txInputs)-1]
			amount.Sub(amount, utxoItem.Amount)
			if needLock {
				uv.UnlockKey(key)
			}
			break
		} else {
			continue
		}
	}
	if it.Error() != nil {
		return nil, nil, nil, it.Error()
	}

	return txInputs, willLockKeys, amount, nil
}

// StochasticApproximationSelectUtxos 随机逼近法选择UTXO
func (uv *UtxoVM) StochasticApproximationSelectUtxos (fromAddr string,  totalNeed *big.Int, needLock, excludeUnconfirmed bool) ([]*protos.TxInput, [][]byte, *big.Int, error)  {

	if totalNeed.Cmp(big.NewInt(0)) == 0 {
		return nil, nil, big.NewInt(0), nil
	}
	curLedgerHeight := uv.ledger.GetMeta().TrunkHeight
	cacheKeys := map[string]bool{}	// 先从cache里找找，不够再从leveldb找,因为leveldb prefix scan比较慢
	utxoTotal := big.NewInt(0)	// 从cache选中的utxo总金额，如果不足再从数据库中继续选择
	willLockKeys := make([][]byte, 0) // 对选中的utxo锁定，如果最后utxo不足，遍历此切片逐一解锁
	foundEnough := false
	txInputs := []*protos.TxInput{}
	uv.clearExpiredLocks()
	uv.UtxoCache.Lock()
	if l2Cache, exist := uv.UtxoCache.Available[fromAddr]; exist {
		for uKey, uItem := range l2Cache {
			if uItem.FrozenHeight > curLedgerHeight || uItem.FrozenHeight == -1 {
				uv.log.Trace("utxo still frozen, skip it", "uKey", uKey, " fheight", uItem.FrozenHeight)
				continue
			}
			refTxid, offset, err := uv.parseUtxoKeys(uKey)
			if err != nil {
				return nil, nil, nil, err
			}
			if needLock {
				if uv.tryLockKey([]byte(uKey)) {
					willLockKeys = append(willLockKeys, []byte(uKey))
				} else {
					uv.log.Debug("can not lock the utxo key, conflict", "uKey", uKey)
					continue
				}
			} else if uv.isLocked([]byte(uKey)) {
				uv.log.Debug("skip locked utxo key", "uKey", uKey)
				continue
			}
			if excludeUnconfirmed { //必须依赖已经上链的tx的UTXO
				isOnChain := uv.ledger.IsTxInTrunk(refTxid)
				if !isOnChain {
					continue
				}
			}
			uv.UtxoCache.Use(fromAddr, uKey)
			utxoTotal.Add(utxoTotal, uItem.Amount)
			txInput := &protos.TxInput{
				RefTxid:      refTxid,
				RefOffset:    int32(offset),
				FromAddr:     []byte(fromAddr),
				Amount:       uItem.Amount.Bytes(),
				FrozenHeight: uItem.FrozenHeight,
			}
			txInputs = append(txInputs, txInput)
			cacheKeys[uKey] = true
			if utxoTotal.Cmp(totalNeed) >= 0 {
				foundEnough = true
				break
			}
		}
	}
	uv.UtxoCache.Unlock()
	if !foundEnough{
		totalNeed.Sub(totalNeed, utxoTotal) // need需要减去cache已经选中的量，【这一步很关键】
		addrPrefix := fmt.Sprintf("%s%s_", pb.UTXOTablePrefix, fromAddr)
		var middleKey []byte
		preFoundUtxoKey, mdOK := uv.PrevFoundKeyCache.Get(fromAddr)
		if mdOK {
			middleKey = preFoundUtxoKey.([]byte)
		}
		it := kvdb.NewQuickIterator(uv.ldb, []byte(addrPrefix), middleKey)
		defer it.Release()
		// utxo 迭代器
		// 通过迭代器之后分为以下类型
		// low:   < totalNeed
		// larger: > totalNeed
		// mid:    = totalNeed
		//
		// low : []*UtxoItem
		// larger: []*UtxoItem                 // 只有最接近的那一个
		// mid:   []*UtxoItem                  // 可以有多个
		// link: map[*UtxoItem]key          // key在构造input时候使用，需要将相关的key记录下来

		low := make([]*UtxoItem, 0)
		larger := make([]*UtxoItem, 0)
		mid := make([]*UtxoItem, 0)
		link := map[*UtxoItem][]byte{}

		for it.Next() {
			key := append([]byte{}, it.Key()...)
			uBinary := it.Value()
			utxoItem := new(UtxoItem)
			// 反序列化utxoItem
			uErr := utxoItem.Loads(uBinary)
			if uErr != nil {
				uv.log.Warn("V__load utxo failed, skipped", "key", key)
				return nil, nil, nil, uErr
			}

			// 过滤
			if _, inCache := cacheKeys[string(key)]; inCache {
				continue // cache命中过，跳过
			}
			if utxoItem.FrozenHeight > curLedgerHeight || utxoItem.FrozenHeight == -1 {
				uv.log.Debug("V__utxo still frozen, skipped", "key", key)
				continue
			}
			// 已经被锁定的utxo跳过
			if uv.isLocked(key) {
				uv.log.Debug("V__utxo locked, skipped", "key", key)
				continue
			}

			// 必须依赖已经上链的tx的UTXO
			refTxid, _, err := uv.parseUtxoKeys(string(key))
			if err != nil {
				return nil, nil, nil, err
			}
			if excludeUnconfirmed { //必须依赖已经上链的tx的UTXO
				isOnChain := uv.ledger.IsTxInTrunk(refTxid)
				if !isOnChain {
					continue
				}
			}

			// 比较
			res := utxoItem.Amount.Cmp(totalNeed)
			if res < 0 {
				low = append(low, utxoItem)
			}else if res > 0 {
				// 保持只有最接近的那一个
				if len(larger) == 0 {
					larger = append(larger, utxoItem)
				}else if utxoItem.Amount.Cmp(larger[0].Amount) < 0  {
					larger[0] = utxoItem
				}
			}else if res == 0 {
				mid = append(mid, utxoItem)
			}

			// 记录 utxoitem <---> key
			// link主要用来获取txid与offset，可以直接使用refTxid, offset, err := uv.parseUtxoKeys(string(key))
			link[utxoItem] = key
		}

		if len(mid) > 0 { // 刚好有一个UTXO是目标值
			refTxid, offset, err := uv.parseUtxoKeys(string(link[mid[0]]))
			if err != nil {
				return nil, nil, nil, err
			}
			txInput := &protos.TxInput{
				RefTxid:      refTxid,
				RefOffset:    int32(offset),
				FromAddr:     []byte(fromAddr),
				Amount:       mid[0].Amount.Bytes(),
				FrozenHeight: mid[0].FrozenHeight,
			}
			txInputs = append(txInputs, txInput)
			// 锁定选中的mid
			uv.tryLockKey(link[mid[0]]) // mid可以由多个，取[0]即可
			willLockKeys = append(willLockKeys, link[mid[0]])
			uv.PrevFoundKeyCache.Add(fromAddr, link[mid[0]])
			return txInputs, willLockKeys, mid[0].Amount, nil
		}

		// 选择utxo
		totalLow := big.NewInt(0)
		// 将零碎UTXO切片low降序排序
		sort.SliceStable(low, func(i, j int) bool {
			return low[i].Amount.Cmp(low[j].Amount) >= 0
		})
		// 遍历计算降序后的low切片UTXO总和
		for _, utxoitem := range low {
			totalLow.Add(totalLow, utxoitem.Amount) // 计算总和
		}

		// 余额不足：totalLow < totalneed && len(larger) == 0
		if totalLow.Cmp(totalNeed) < 0 && len(larger) == 0{
			return nil, nil, nil, ErrNoEnoughUTXO // 余额不足
		}else if totalLow.Cmp(totalNeed) < 0 && len(larger) > 0 {
			// 零碎UTXO总和不足发起交易，但是有超过目标值的大面额UTXO则直接使用该大面值UTXO
			refTxid, offset, err := uv.parseUtxoKeys(string(link[larger[0]]))
			if err != nil {
				return nil, nil, nil, err
			}
			txInput := &protos.TxInput{
				RefTxid:      refTxid,
				RefOffset:    int32(offset),
				FromAddr:     []byte(fromAddr),
				Amount:       larger[0].Amount.Bytes(),
				FrozenHeight: larger[0].FrozenHeight,
			}
			txInputs = append(txInputs[:0], txInput) // 前面的cache选择utxo时可能有将数据存在txInputs中，对于此if这里需要先清空
			if needLock {
				if uv.tryLockKey(link[larger[0]]) { // 将选中的utxo即larger[0]锁定
					willLockKeys = append(willLockKeys[:0], link[larger[0]])
				} else {
					uv.log.Debug("V__can not lock the utxo key, conflict", "key", link[larger[0]])
				}
			}
			uv.PrevFoundKeyCache.Add(fromAddr, link[larger[0]])
			return txInputs, willLockKeys, larger[0].Amount, nil
		}else if totalLow.Cmp(totalNeed) == 0 {
			// 零碎的UTXO总和刚好为目标值
			for _, utxoitem := range low {
				refTxid, offset, err := uv.parseUtxoKeys(string(link[utxoitem]))
				if err != nil {
					return nil, nil, nil, err
				}
				if needLock{
					if uv.tryLockKey(link[utxoitem]) { // 注意lock不能在迭代的时候进行
						willLockKeys = append(willLockKeys, link[utxoitem])
					}else {
						uv.log.Debug("V__can not lock the utxo key, conflict", "key", link[utxoitem])
						//total.Sub(total, utxoitem.Amount)
						//continue
					}
				}
				txInput := &protos.TxInput{
					RefTxid: refTxid,
					RefOffset: int32(offset),
					FromAddr: []byte(fromAddr),
					Amount: utxoitem.Amount.Bytes(),
					FrozenHeight: utxoitem.FrozenHeight,
				}
				txInputs = append(txInputs, txInput)
			}
			uv.PrevFoundKeyCache.Add(fromAddr, link[low[len(low)-1]])
			return txInputs, willLockKeys, totalLow, nil
		}else { // 零碎UTXO总和超过目标值（另分有无超过目标值的大面额UTXO两种情况）
			// Solve subset sum by stochastic approximation，随机逼近法挑零碎的UTXO组合
			vfIncluded := make([]bool, 0) // 排除标志符
			vfBest := make([]bool, 0) // 最优记录标志符
			for range low {
				vfIncluded = append(vfIncluded, false)
				vfBest = append(vfBest, true) // vfBest最初全为true，即先默认需要所有的零碎UTXO来组成最适输入
			}
			nBest := totalLow // 最优值，该值将动态调整（totalLow由所有零碎UTXO合计得到，映证最初的vfBest元素需要都为true）
			// 随机逼近
			/*算法可能out of memory，还需要优化
			testFlag := 0
			start := time.Now()
			fmt.Println("for start", start)*/
			for nRep := 0; nRep < 100 && nBest != totalNeed; nRep++ { // 操作1000次，如果最优值等于目标值或者1000次结束，则退出循环
				for i := range low { // 排除标志符设为false
					vfIncluded[i] = false
				}
				nTotal := big.NewInt(0)  // 总数为0
				fReachedTarget := false // 找到合适对象设为false
				for nPass := 0; nPass < 2 && !fReachedTarget; nPass++ { // 进行两次操作，当两次操作完成或者找到合适对象时退出循环
					for i := 0; i < len(low); i++ { // 遍历所有小于目标值的对象
						/*testFlag ++*/
						// go没有`if (nPass == 0 ? rand() % 2 : !vfIncluded[i])`这样的写法
						addiction := false
						if nPass == 0 {
							if rand.New(rand.NewSource(time.Now().UnixNano())).Int() % 2 != 0 {
								addiction = true
							}
						}else {
							addiction = !vfIncluded[i]// && !uv.isLocked(link[low[i]]) // 过滤锁定？
						}
						if addiction { // 第一次操作采取随机抽取，第二次操作针对第一次没有随机抽取到的对象操作
							nTotal.Add(nTotal, low[i].Amount) // 累加值
							vfIncluded[i] = true // 标识为已使用
							if nTotal.Cmp(totalNeed) >= 0{ // 如果总数大于目标值
								fReachedTarget = true // 发现目标
								if nTotal.Cmp(nBest) < 0{ // 该目标比以往发现的目标好
									nBest.Set(nTotal) // 替换为最好目标
									copy(vfBest, vfIncluded) // 深拷贝，保存相应的对象
								}
								nTotal.Sub(nTotal, low[i].Amount) // 放弃该对象，寻找下一对象
								vfIncluded[i] = false // 该对象值为false，便于第二次随机抽取使用
							}
						}
					}
				}
			}
			/*stop := time.Now()
			fmt.Println("for done", stop, "花费时间", stop.Sub(start).Seconds())
			fmt.Println("循环次数", testFlag)
			fmt.Println("slice size", len(vfBest))*/
			//fmt.Println("占用内存include&best", vfIncluded, vfBest)
			// larger存在且其UTXO比随机挑出的零碎UTXO组合更优，直接使用larger
			if len(larger) > 0 && larger[0].Amount.Cmp(nBest) < 0 /*.Int64() <= nBest.Int64()*/ { // 最大值对象比累加对象要好
				refTxid, offset, err := uv.parseUtxoKeys(string(link[larger[0]]))
				if err != nil {
					return nil, nil, nil, err
				}
				txInput := &protos.TxInput{
					RefTxid:      refTxid,
					RefOffset:    int32(offset),
					FromAddr:     []byte(fromAddr),
					Amount:       larger[0].Amount.Bytes(),
					FrozenHeight: larger[0].FrozenHeight,
				}
				txInputs = append(txInputs, txInput)
				if needLock{ // 锁定
					if uv.tryLockKey(link[larger[0]]) { // 注意lock不能在迭代的时候进行
						willLockKeys = append(willLockKeys, link[larger[0]])
					}else {
						uv.log.Debug("V__can not lock the utxo key, conflict", "key", link[larger[0]])
					}
				}
				uv.PrevFoundKeyCache.Add(fromAddr, link[larger[0]])
				// larger[0].Amount不需要加上在cache中已经累加的utxoTotal
				// totalNeed <= larger[0].Amount < nBest+utxoTotal
				return txInputs, willLockKeys, larger[0].Amount/*.Add(larger[0].Amount, utxoTotal)*/, nil
			}else { // larger存在但其UTXO比随机挑出的零碎UTXO组合差，使用零碎组合
				lastI := 0
				for i := 0; i < len(low); i++ {
					// 循环候选对象构建交易输入
					if vfBest[i]{ // 取出合适对象
						refTxid, offset, err := uv.parseUtxoKeys(string(link[low[i]]))
						if err != nil {
							return nil, nil, nil, err
						}
						txInput := &protos.TxInput{
							RefTxid:      refTxid,
							RefOffset:    int32(offset),
							FromAddr:     []byte(fromAddr),
							Amount:       low[i].Amount.Bytes(),
							FrozenHeight: low[i].FrozenHeight,
						}
						txInputs = append(txInputs, txInput)
						if needLock{ // 锁定
							if uv.tryLockKey(link[low[i]]) {
								willLockKeys = append(willLockKeys, link[low[i]])
							}else {
								uv.log.Debug("V__can not lock the utxo key, conflict", "key", link[low[i]])
							}
						}
						lastI = i
					}
				}
				uv.PrevFoundKeyCache.Add(fromAddr, link[low[lastI]])
				//// debug print
				/*fmt.Println("best and need", nBest, totalNeed.Int64(), "and the low slice")
				for i, item := range low {
					txid, _, _ := uv.parseUtxoKeys(string(link[item]))
					fmt.Println(i, item.Amount,"<-amount, bool->\t", vfBest[i], "\tkey", hex.EncodeToString(txid))
				}*/
				/*fmt.Println("cache+low完成utxo选择", utxoTotal)
				t := big.NewInt(0)
				for i, input := range txInputs {
					t.Add(t, big.NewInt(0).SetBytes(input.Amount))
					fmt.Println(i, "amount\t", big.NewInt(0).SetBytes(input.Amount))
					fmt.Println(i, "ref txid\t", hex.EncodeToString(input.RefTxid))
					fmt.Println(i, "offset\t", input.RefOffset)
					fmt.Println(i, "from\t", string(input.FromAddr))
				}
				fmt.Printf("total: %d选出的nBest%d\n\n", t.Int64(), nBest.Int64())*/
				return txInputs, willLockKeys, nBest.Add(nBest, utxoTotal), nil // best需要再加上cache中已经累计的total
			}
		}
	}
	return txInputs, willLockKeys, utxoTotal, nil
}