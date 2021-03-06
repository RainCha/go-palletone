/*
 *
 *     This file is part of go-palletone.
 *     go-palletone is free software: you can redistribute it and/or modify
 *     it under the terms of the GNU General Public License as published by
 *     the Free Software Foundation, either version 3 of the License, or
 *     (at your option) any later version.
 *     go-palletone is distributed in the hope that it will be useful,
 *     but WITHOUT ANY WARRANTY; without even the implied warranty of
 *     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *     GNU General Public License for more details.
 *     You should have received a copy of the GNU General Public License
 *     along with go-palletone.  If not, see <http://www.gnu.org/licenses/>.
 * /
 *
 *  * @author PalletOne core developers <dev@pallet.one>
 *  * @date 2018
 *
 */

package storage

import (
	"github.com/palletone/go-palletone/common/rlp"
	"github.com/palletone/go-palletone/dag/constants"
	"github.com/palletone/go-palletone/dag/modules"
	"strings"
)

func (statedb *StateDb) SaveContractTemplate(templateId []byte, bytecode []byte, version []byte) error {
	key := append(constants.CONTRACT_TPL, templateId...)
	key = append(key, []byte(modules.FIELD_SPLIT_STR)...)
	key = append(key, []byte(modules.FIELD_TPL_BYTECODE)...)
	key = append(key, []byte(modules.FIELD_SPLIT_STR)...)
	key = append(key, version...)
	if err := StoreBytes(statedb.db, key, bytecode); err != nil {
		return err
	}
	return nil
}

/**
获取模板所有属性
To get contract or contract template all fields and return
*/
func (statedb *StateDb) GetTplAllState(id []byte) []*modules.ContractReadSet {
	// key format: [PREFIX][ID]_[field]_[version]
	key := append(constants.CONTRACT_TPL, id...)
	key = append(key, []byte(modules.FIELD_SPLIT_STR)...)
	data := getprefix(statedb.db, []byte(key))
	if data == nil || len(data) <= 0 {
		return nil
	}
	allState := []*modules.ContractReadSet{}
	for k, v := range data {
		sKey := strings.Split(k, "^*^")
		if len(sKey) != 3 {
			continue
		}
		var version modules.StateVersion
		if !version.ParseStringKey(k) {
			continue
		}
		rdSet := &modules.ContractReadSet{
			Key:     sKey[1],
			Version: &version,
			Value:   v,
		}
		allState = append(allState, rdSet)
	}
	return allState
}

/**
获取合约（或模板）某一个属性
To get contract or contract template one field
*/
func (statedb *StateDb) GetTplState(id []byte, field string) (*modules.StateVersion, []byte) {
	//key := fmt.Sprintf("%s%s^*^%s^*^", CONTRACT_TPL, hexutil.Encode(id[:]), field)
	key := append(constants.CONTRACT_TPL, id...)
	key = append(key, []byte(modules.FIELD_SPLIT_STR)...)
	key = append(key, []byte(field)...)
	data := getprefix(statedb.db, []byte(key))
	if data == nil || len(data) != 1 {
		return nil, nil
	}
	for k, v := range data {
		var version modules.StateVersion
		if !version.ParseStringKey(k) {
			return nil, nil
		}
		return &version, v
	}
	return nil, nil
}

/**
获取合约模板
To get contract template
*/
func (statedb *StateDb) GetContractTpl(templateID []byte) (version *modules.StateVersion, bytecode []byte, name string, path string) {
	key := append(constants.CONTRACT_TPL, templateID...)
	key = append(key, []byte(modules.FIELD_SPLIT_STR)...)
	key = append(key, []byte(modules.FIELD_TPL_BYTECODE)...)
	data := statedb.GetPrefix(key)

	if len(data) == 1 {
		for _, v := range data {
			if err := rlp.DecodeBytes(v, &bytecode); err != nil {
				statedb.logger.Error("GetContractTpl when get bytecode", "error", err.Error(), "codeing:", v, "val:", bytecode)
				return
			}
		}
	}

	version, nameByte := statedb.GetTplState(templateID, modules.FIELD_TPL_NAME)
	if nameByte == nil {
		return
	}
	if err := rlp.DecodeBytes(nameByte, &name); err != nil {
		statedb.logger.Error("GetContractTpl when get name", "error", err.Error())
		return
	}

	_, pathByte := statedb.GetTplState(templateID, modules.FIELD_TPL_PATH)
	if err := rlp.DecodeBytes(pathByte, &path); err != nil {
		statedb.logger.Error("GetContractTpl when get path", "error", err.Error())
		return
	}
	return
}
