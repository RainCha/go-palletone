package list

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/palletone/go-palletone/core/vmContractPub/flogging"
	"github.com/pkg/errors"
	"sync"
)

var logger = flogging.MustGetLogger("cclist")

type CCInfo struct {
	Id      []byte
	Name    string
	Path    string
	Version string

	SysCC  bool
	Enable bool
}

type chain struct {
	Version int
	CClist  map[string]*CCInfo //chainCodeId
}

var chains = struct {
	mu    sync.Mutex
	Clist map[string]*chain //chainId
}{Clist: make(map[string]*chain)}

func chainsInit() {
	chains.Clist = nil
	chains.Clist = make(map[string]*chain)
}

func addChainCodeInfo(c *chain, cc *CCInfo) error {
	if c == nil || cc == nil {
		return errors.New("chain or ccinfo is nil")
	}

	for k, v := range c.CClist {
		if k == cc.Name && v.Version == cc.Version {
			logger.Errorf("chaincode [%s] , version[%s] already exit, %v", cc.Name, cc.Version, v)
			return errors.New("already exit chaincode")
		}
	}
	c.CClist[cc.Name] = cc

	return nil
}

func SetChaincode(cid string, version int, chaincode *CCInfo) error {
	chains.mu.Lock()
	defer chains.mu.Unlock()

	logger.Infof("chainId[%s] ,%d, chaincode[%s]id[%s]", cid, version, chaincode.Name, hex.EncodeToString(chaincode.Id))
	for k, v := range chains.Clist {
		if k == cid {
			logger.Infof("chainId[%s] already exit, %v", cid, v)

			return addChainCodeInfo(v, chaincode)
		}
	}
	cNew := chain{
		Version: version,
		CClist:  make(map[string]*CCInfo),
	}
	chains.Clist[cid] = &cNew

	return addChainCodeInfo(&cNew, chaincode)
}

func GetChaincodeList(cid string) (*chain, error) {
	if cid == "" {
		return nil, errors.New("param is nil")
	}

	if chains.Clist[cid] != nil {
		return chains.Clist[cid], nil
	}
	errmsg := fmt.Sprintf("not find chainId[%s] in chains", cid)

	return nil, errors.New(errmsg)
}

func GetChaincode(cid string, deployId []byte) (*CCInfo, error) {
	if cid == "" {
		return nil, errors.New("param is nil")
	}
	if chains.Clist[cid] != nil {
		clist := chains.Clist[cid]
		for _, v := range clist.CClist {
			//logger.Infof("++++:%v",  *v)
			//logger.Infof("find,%s, id[%s]--[%s], ", v.Name, hex.EncodeToString(v.Id), hex.EncodeToString(deployId))
			if bytes.Equal(v.Id, deployId) == true {
				//logger.Infof("++++++++++++++++find,%s", v.Name)
				return v, nil
			}
		}
	}
	errmsg := fmt.Sprintf("not find chainId[%s], deployId[%v] in chains", cid, deployId)
	return nil, errors.New(errmsg)
}

func DelChaincode(cid string, ccName string, version string) error {
	chains.mu.Lock()
	defer chains.mu.Unlock()

	if cid == "" || ccName == "" {
		return errors.New("param is nil")
	}

	if chains.Clist[cid] != nil {
		delete(chains.Clist[cid].CClist, ccName)
		logger.Infof("del chaincode[%s]", ccName)
		return nil
	}
	logger.Infof("not find chaincode[%s]", ccName)

	return nil
}

func init() {
	chainsInit()
}
