/*
   This file is part of go-palletone.
   go-palletone is free software: you can redistribute it and/or modify
   it under the terms of the GNU General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.
   go-palletone is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU General Public License for more details.
   You should have received a copy of the GNU General Public License
   along with go-palletone.  If not, see <http://www.gnu.org/licenses/>.
*/
/*
 * @author PalletOne core developer Albert·Gou <dev@pallet.one>
 * @date 2018
 */

package mediatorplugin

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/palletone/go-palletone/common"
	"github.com/palletone/go-palletone/common/event"
	"github.com/palletone/go-palletone/common/log"
	"github.com/palletone/go-palletone/core/accounts"
)

var (
	errTerminated = errors.New("terminated")
)

func newChainBanner(dag iDag) {
	fmt.Printf("\n" +
		"*   ------- NEW CHAIN -------   *\n" +
		"*   - Welcome to PalletOne! -   *\n" +
		"*   -------------------------   *\n" +
		"\n")

	if dag.GetSlotAtTime(time.Now()) > 200 {
		fmt.Printf("Your genesis seems to have an old timestamp\n" +
			"Please consider using the --genesistime option to give your genesis a recent timestamp\n" +
			"\n")
	}
}

func (mp *MediatorPlugin) SubscribeNewUnitEvent(ch chan<- NewUnitEvent) event.Subscription {
	return mp.newUnitScope.Track(mp.newUnitFeed.Subscribe(ch))
}

func (mp *MediatorPlugin) scheduleProductionLoop() {
	// 1. 计算下一秒的滴答时刻，如果少于50毫秒，则多等一秒开始
	now := time.Now()
	timeToNextSecond := time.Second - time.Duration(now.Nanosecond())
	if timeToNextSecond < 50*time.Millisecond {
		timeToNextSecond += time.Second
	}

	nextWakeup := now.Add(timeToNextSecond)

	// 2. 安排验证单元生产循环
	// production unit until termination is requested
	select {
	case <-mp.quit:
		return
	default:
		go mp.UnitProductionLoop(nextWakeup)
	}
}

//验证单元生产状态类型
type ProductionCondition uint8

//验证单元生产状态枚举
const (
	Produced ProductionCondition = iota // 正常生产验证单元
	NotSynced
	NotMyTurn
	NotTimeYet
	NoPrivateKey
	//	LowParticipation
	Lag
	//	Consecutive
	ExceptionProducing
	UnknownCondition
)

func (mp *MediatorPlugin) UnitProductionLoop(wakeup time.Time) ProductionCondition {
	// Start to production unit for expiration
	time.Sleep(wakeup.Sub(time.Now()))

	// 1. 尝试生产验证单元
	result, detail := mp.MaybeProduceUnit()

	// 2. 打印尝试结果
	switch result {
	case Produced:
		log.Info("Generated Unit #" + detail["Num"] + " hash: " + detail["Hash"] +
			" with timestamp " + detail["Timestamp"] + " by mediator: " + detail["Mediator"])
	case NotSynced:
		log.Info("Not producing Unit because production is disabled " +
			"until we receive a recent Unit (see: --enable-stale-production)")
	case NotTimeYet:
		//log.Debug("Not producing Unit because next slot time is " + detail["NextTime"] +
		//	" , but now is " + detail["Now"])
	case NotMyTurn:
		//log.Debug("Not producing Unit because current scheduled mediator is " +
		//	detail["ScheduledMediator"])
	case Lag:
		log.Info("Not producing Unit because node didn't wake up within 500ms of the slot time." +
			" Scheduled Time is: " + detail["ScheduledTime"] + ", but now is " + detail["Now"])
	case NoPrivateKey:
		log.Info("Not producing Unit because I don't have the private key for " +
			detail["ScheduledKey"])
	case ExceptionProducing:
		log.Info("Exception producing unit")
	case UnknownCondition:
		log.Info("Unknown condition!")
	}

	// 3. 继续循环生产计划
	mp.scheduleProductionLoop()

	return result
}

func (mp *MediatorPlugin) MaybeProduceUnit() (ProductionCondition, map[string]string) {
	//	println("\n尝试生产验证单元...")
	detail := map[string]string{}

	dag := mp.dag

	// 整秒调整，四舍五入
	nowFine := time.Now()
	now := time.Unix(nowFine.Add(500*time.Millisecond).Unix(), 0)

	// 1. 判断是否满足生产的各个条件
	nextSlotTime := dag.GetSlotTime(1)
	// If the next Unit production opportunity is in the present or future, we're synced.
	if !mp.productionEnabled {
		if nextSlotTime.After(now) || nextSlotTime.Equal(now) {
			mp.productionEnabled = true
		} else {
			return NotSynced, detail
		}
	}

	slot := dag.GetSlotAtTime(now)
	// is anyone scheduled to produce now or one second in the future?
	if slot == 0 {
		detail["NextTime"] = nextSlotTime.Format("2006-01-02 15:04:05")
		detail["Now"] = now.Format("2006-01-02 15:04:05")
		return NotTimeYet, detail
	}

	// this Conditional judgment should fail, because now <= HeadUnitTime
	// should have resulted in slot == 0.
	//
	// if this assert triggers, there is a serious bug in GetSlotAtTime()
	// which would result in allowing a later block to have a timestamp
	// less than or equal to the previous Unit
	if !(now.Unix() > dag.HeadUnitTime()) {
		panic("\n The later Unit have a timestamp less than or equal to the previous!")
	}

	scheduledMediator := dag.GetScheduledMediator(slot)
	if scheduledMediator.Equal(common.Address{}) {
		log.Error("The current shuffled mediators is nil!")
		return UnknownCondition, detail
	}

	// we must control the Mediator scheduled to produce the next Unit.
	med, ok := mp.mediators[scheduledMediator]
	if !ok {
		detail["ScheduledMediator"] = scheduledMediator.Str()
		return NotMyTurn, detail
	}

	// 此处应该判断scheduledMediator的签名公钥对应的私钥在本节点是否存在
	ks := mp.ptn.GetKeyStore()
	err := ks.Unlock(accounts.Account{Address: scheduledMediator}, med.Password)
	if err != nil {
		detail["ScheduledKey"] = scheduledMediator.Str()
		return NoPrivateKey, detail
	}

	scheduledTime := dag.GetSlotTime(slot)
	diff := scheduledTime.Sub(now)
	if diff > 500*time.Millisecond || diff < -500*time.Millisecond {
		detail["ScheduledTime"] = scheduledTime.Format("2006-01-02 15:04:05")
		detail["Now"] = now.Format("2006-01-02 15:04:05")
		return Lag, detail
	}

	// 2. 生产验证单元
	newUnit := dag.GenerateUnit(scheduledTime, scheduledMediator, ks, mp.ptn.TxPool())
	if newUnit.IsEmpty() {
		return ExceptionProducing, detail
	}

	unitHash := newUnit.UnitHash
	detail["Num"] = strconv.FormatUint(newUnit.NumberU64(), 10)
	time := time.Unix(newUnit.Timestamp(), 0)
	detail["Timestamp"] = time.Format("2006-01-02 15:04:05")
	detail["Mediator"] = scheduledMediator.Str()
	detail["Hash"] = unitHash.Hex()

	// 3. 初始化签名unit相关的签名分片的buf
	mp.initTBLSRecoverBuf(scheduledMediator, unitHash)

	// 4. 异步向区块链网络广播验证单元
	// todo 后面改为由p2p转发
	mp.addToTBLSSignBuf(newUnit)
	mp.newUnitFeed.Send(NewUnitEvent{Unit: newUnit})

	return Produced, detail
}

func (mp *MediatorPlugin) initTBLSRecoverBuf(localMed common.Address, newUnitHash common.Hash) {
	aSize := mp.dag.GetActiveMediatorCount()
	mp.toTBLSRecoverBuf[localMed][newUnitHash] = newSigShareSet(aSize)
}
