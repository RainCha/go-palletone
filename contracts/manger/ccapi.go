package manger

import (
	"fmt"
	"golang.org/x/net/context"
	"time"

	"bytes"
	"container/list"
	"encoding/hex"
	"encoding/json"
	cp "github.com/palletone/go-palletone/common/crypto"
	db "github.com/palletone/go-palletone/contracts/comm"
	cclist "github.com/palletone/go-palletone/contracts/list"
	"github.com/palletone/go-palletone/contracts/modules"
	"github.com/palletone/go-palletone/contracts/scc"
	"github.com/palletone/go-palletone/contracts/ucc"
	"github.com/palletone/go-palletone/core/vmContractPub/crypto"
	"github.com/palletone/go-palletone/core/vmContractPub/protos/peer"
	pb "github.com/palletone/go-palletone/core/vmContractPub/protos/peer"
	"github.com/palletone/go-palletone/dag"
	unit "github.com/palletone/go-palletone/dag/modules"
	"github.com/pkg/errors"
)

var debugX bool = true

type TempCC struct {
	templateId []byte
	name       string
	path       string
	vers       string
}

var listCC list.List

func listAdd(cc *TempCC) error {
	if cc != nil {
		//fmt.Printf("==name[%s]", cc.name)
		listCC.PushBack(*cc)
	}
	return nil
}

func listDel(templateId []byte) {
	for e := listCC.Front(); e != nil; e = e.Next() {
		if bytes.Equal(e.Value.(TempCC).templateId, templateId) == true {
			listCC.Remove(e)
		}
	}
}

func listGet(templateId []byte) (*TempCC, error) {
	logger.Infof("listGet [%v]", templateId)
	for e := listCC.Front(); e != nil; e = e.Next() {
		if bytes.Equal(e.Value.(TempCC).templateId, templateId) == true {
			cc := &TempCC{
				templateId: templateId,
				name:       e.Value.(TempCC).name,
				path:       e.Value.(TempCC).path,
				vers:       e.Value.(TempCC).vers,
			}
			//fmt.Printf("==name[%s]", cc.name)
			return cc, nil
		}
	}
	return nil, errors.New("not find")
}

// contract manger module init
func Init(dag dag.IDag) error {
	if err := db.SetCcDagHand(dag); err != nil {
		return err
	}
	if err := peerServerInit(); err != nil {
		logger.Errorf("peerServerInit:%s", err)
		return err
	}
	if err := systemContractInit(); err != nil {
		logger.Errorf("systemContractInit error:%s", err)
		return err
	}
	logger.Info("contract manger init ok")

	return nil
}

func InitNoSysCCC() error {
	if err := peerServerInit(); err != nil {
		logger.Errorf("peerServerInit error:%s", err)
		return err
	}
	return nil
}

func Deinit() error {
	if err := peerServerDeInit(); err != nil {
		logger.Errorf("peerServerDeInit error:%s", err)
		return err
	}

	if err := systemContractDeInit(); err != nil {
		logger.Errorf("systemContractDeInit error:%s", err)
		return err
	}
	return nil
}

func GetSysCCList() (ccInf []cclist.CCInfo, ccCount int, errs error) {
	scclist := make([]cclist.CCInfo, 0)
	ci := cclist.CCInfo{}

	cclist, count, err := scc.SysCCsList()
	for _, ccinf := range cclist {
		ci.Name = ccinf.Name
		ci.Path = ccinf.Path
		ci.Enable = ccinf.Enabled
		ci.SysCC = true
		scclist = append(scclist, ci)
	}
	return scclist, count, err
}

func GetUsrCCList() {
}

//install but not into db
func Install(dag dag.IDag, chainID string, ccName string, ccPath string, ccVersion string) (payload *unit.ContractTplPayload, err error) {
	logger.Infof("==========install enter=======")
	logger.Infof("name[%s]path[%s]version[%s]", ccName, ccPath, ccVersion)
	defer logger.Infof("-----------install exit--------")
	usrcc := &ucc.UserChaincode{
		Name:    ccName,
		Path:    ccPath,
		Version: ccVersion,
		Enabled: true,
	}

	paylod, err := ucc.GetUserCCPayload(chainID, usrcc)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	buffer.Write([]byte(ccName))
	buffer.Write([]byte(ccPath))
	buffer.Write([]byte(ccVersion))
	tpid := cp.Keccak256Hash(buffer.Bytes())

	payloadUnit := &unit.ContractTplPayload{
		TemplateId: []byte(tpid[:]),
		Name:       ccName,
		Path:       ccPath,
		Version:    ccVersion,
		Bytecode:   paylod,
	}

	//test
	tcc := &TempCC{templateId: []byte(tpid[:]), name: ccName, path: ccPath, vers: ccVersion}
	listAdd(tcc)
	logger.Infof("template id [%v]", tcc.templateId)

	return payloadUnit, nil
}

func DeployByName(idag dag.IDag, chainID string, txid string, ccName string, ccPath string, ccVersion string, args [][]byte, timeout time.Duration) (depllyId []byte, respPayload *peer.ContractDeployPayload, e error) {
	var mksupt Support = &SupportImpl{}
	setChainId := "palletone"
	setTimeOut := time.Duration(30) * time.Second

	if chainID != "" {
		setChainId = chainID
	}
	if timeout > 0 {
		setTimeOut = timeout
	}
	if txid == "" || ccName == "" || ccPath == "" {
		return nil, nil, errors.New("input param is nil")
	}
	randNum, err := crypto.GetRandomNonce()
	if err != nil {
		return nil, nil, errors.New("crypto.GetRandomNonce error")
	}

	txsim, err := mksupt.GetTxSimulator(idag, chainID, txid)
	if err != nil {
		return nil, nil, errors.New("GetTxSimulator error")
	}
	//randNum, err := crypto.GetRandomNonce()
	//if err != nil {
	//	return nil, nil, errors.New("crypto.GetRandomNonce error")
	//}

	usrcc := &ucc.UserChaincode{
		Name:     ccName,
		Path:     ccPath,
		Version:  ccVersion,
		InitArgs: args,
		Enabled:  true,
	}
	spec := &pb.ChaincodeSpec{
		Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]),
		Input: &pb.ChaincodeInput{
			Args: args,
		},
		ChaincodeId: &pb.ChaincodeID{
			Name:    ccName,
			Path:    ccPath,
			Version: ccVersion,
		},
	}

	//err = ucc.DeployUserCC(spec, setChainId, usrcc, txid, txsim, setTimeOut)
	err = ucc.DeployUserCC(spec, setChainId, usrcc, txid, txsim, setTimeOut)
	if err != nil {
		return nil, nil, errors.New("Deploy fail")
	}

	cc := &cclist.CCInfo{
		Id:      randNum,
		Name:    ccName,
		Path:    ccPath,
		Version: ccVersion,
		SysCC:   false,
		Enable:  true,
	}
	err = cclist.SetChaincode(setChainId, 0, cc)
	if err != nil {
		logger.Errorf("setchaincode[%s]-[%s] fail", setChainId, cc.Name)
	}

	return cc.Id, nil, err
}

func Deploy(idag dag.IDag, chainID string, templateId []byte, txid string, args [][]byte, timeout time.Duration) (deployId []byte, deployPayload *unit.ContractDeployPayload, e error) {
	logger.Infof("==========Deploy enter=======")
	defer logger.Infof("-----------Deploy exit--------")
	logger.Infof("chainid[%s]templateId[%s]txid[%s]", chainID, hex.EncodeToString(templateId), txid)
	var mksupt Support = &SupportImpl{}
	setChainId := "palletone"
	setTimeOut := time.Duration(30) * time.Second

	if chainID != "" {
		setChainId = chainID
	}
	if timeout > 0 {
		setTimeOut = timeout
	}
	spec := &pb.ChaincodeSpec{
		Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]),
		Input: &pb.ChaincodeInput{
			Args: args,
		},
	}
	templateCC, err := ucc.RecoverChainCodeFromDb(spec, chainID, templateId)
	if err != nil {
		logger.Errorf("chainid[%s]-templateId[%v], RecoverChainCodeFromDb fail:%s", chainID, templateId, err)
		return nil, nil, err
	}

	//test!!!!!!
	//todo del
	if txid == "" || templateCC.Name == "" || templateCC.Path == "" {
		logger.Errorf("cc param is null")
		//test
		tmpcc, err := listGet(templateId)
		if err == nil {
			templateCC.Name = tmpcc.name
			templateCC.Path = tmpcc.path
			templateCC.Version = tmpcc.vers
		}
	}
	txsim, err := mksupt.GetTxSimulator(idag, chainID, txid)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "GetTxSimulator error")
	}
	randNum, err := crypto.GetRandomNonce()
	if err != nil {
		return nil, nil, errors.WithMessage(err, "crypto.GetRandomNonce error")
	}

	usrccName := templateCC.Name + "-" + hex.EncodeToString(randNum) //[0:8]
	//usrccName := templateCC.Name //[0:8]
	usrcc := &ucc.UserChaincode{
		Name:     usrccName,
		Path:     templateCC.Path,
		Version:  templateCC.Version,
		InitArgs: args,
		Enabled:  true,
	}

	chaincodeID := &pb.ChaincodeID{
		Name:    usrcc.Name,
		Path:    usrcc.Path,
		Version: usrcc.Version,
	}
	spec.ChaincodeId = chaincodeID
	err = ucc.DeployUserCC(spec, setChainId, usrcc, txid, txsim, setTimeOut)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "Deploy fail")
	}
	cc := &cclist.CCInfo{
		Id:      randNum,
		Name:    usrccName,
		Path:    templateCC.Path,
		Version: templateCC.Version,
		SysCC:   false,
		Enable:  true,
	}
	err = cclist.SetChaincode(setChainId, 0, cc)
	if err != nil {
		logger.Errorf("setchaincode[%s]-[%s] fail", setChainId, cc.Name)
	}

	unit, err := RwTxResult2DagDeployUnit(txsim, templateId, txid, cc.Name, cc.Id, args, timeout)
	if err != nil {
		logger.Errorf("chainID[%s] converRwTxResult2DagUnit failed", chainID)
		return nil, nil, errors.WithMessage(err, "Conver RwSet to dag unit fail")
	}
	return cc.Id, unit, err
}

//timeout:ms
// ccName can be contract Id
//func Invoke(chainID string, deployId []byte, txid string, args [][]byte, timeout time.Duration) (*peer.ContractInvokePayload, error) {
func Invoke(contractid []byte, idag dag.IDag, chainID string, deployId []byte, txid string, tx *unit.Transaction, args [][]byte, timeout time.Duration) (*modules.ContractInvokeResult, error) {
	logger.Infof("==========Invoke enter=======")
	logger.Infof("deployId[%s] txid[%s]", hex.EncodeToString(deployId), txid)
	defer logger.Infof("-----------Invoke exit--------")

	var mksupt Support = &SupportImpl{}
	creator := []byte("palletone")

	cc, err := cclist.GetChaincode(chainID, deployId)
	if err != nil {
		return nil, err
	}
	if cc.Name == "" {
		errstr := fmt.Sprintf("chainCode[%v] not deplay!!", deployId)
		return nil, errors.New(errstr)
	}

	fullArgs := [][]byte{}
	//TODO parse tx and make another args include InvokeAddr, InvokeFee, InvokeToken
	// txin := unit.Input{
	// 	PreviousOutPoint: &unit.OutPoint{
	// 		TxHash:       common.Hash{},
	// 		MessageIndex: 1234,
	// 		OutIndex:     12344,
	// 	},
	// 	SignatureScript: []byte("1234567890"),
	// 	Extra:           []byte("990019202020"),
	// }
	// txout := unit.Output{
	// 	Value:    10000,
	// 	PkScript: []byte("kssssssssssssssssssslsll"),
	// 	Asset: &unit.Asset{
	// 		AssetId:  unit.PTNCOIN,
	// 		UniqueId: unit.PTNCOIN,
	// 		ChainId:  1,
	// 	},
	// }
	// payment := &unit.PaymentPayload{
	// 	Inputs:   []*unit.Input{&txin},
	// 	Outputs:  []*unit.Output{&txout},
	// 	LockTime: 12,
	// }
	// tx2 := &unit.Transaction{
	// 	TxMessages: []*unit.Message{
	// 		{
	// 			App:     unit.APP_PAYMENT,
	// 			Payload: payment,
	// 		},
	// 	},
	// }
	// tx2.TxHash = tx2.Hash()
	msg0 := tx.TxMessages[0].Payload.(*unit.PaymentPayload)
	invokeAddr, _ := idag.GetAddrByOutPoint(msg0.Inputs[0].PreviousOutPoint)
	invokeTokens := unit.InvokeTokens{}
	outputs := msg0.Outputs
	invokeTokens.Asset = *outputs[0].Asset
	for _, output := range outputs {
		invokeTokens.Amount += output.Value
	}
	invokeFees, _ := idag.GetTxFee(tx)
	invokeInfo := unit.InvokeInfo{
		InvokeAddress: invokeAddr,
		InvokeTokens:  invokeTokens,
		InvokeFees:    invokeFees,
	}
	invokeInfoBytes, err := json.Marshal(invokeInfo)
	fullArgs = append(fullArgs, invokeInfoBytes)

	fullArgs = append(fullArgs, args...)
	logger.Infof("Invoke [%s][%s]", chainID, cc.Name)
	start := time.Now()
	es := NewEndorserServer(mksupt)
	spec := &pb.ChaincodeSpec{
		ChaincodeId: &pb.ChaincodeID{Name: cc.Name},
		Type:        pb.ChaincodeSpec_GOLANG,
		Input:       &pb.ChaincodeInput{Args: fullArgs},
	}

	cid := &pb.ChaincodeID{
		Path:    "", //no use
		Name:    cc.Name,
		Version: cc.Version,
	}

	sprop, prop, err := signedEndorserProposa(chainID, txid, spec, creator, []byte("msg1"))
	if err != nil {
		logger.Errorf("signedEndorserProposa error[%v]", err)
		return nil, err
	}

	rsp, unit, err := es.ProcessProposal(contractid, idag, deployId, context.Background(), sprop, prop, chainID, cid, timeout)
	t0 := time.Now()
	duration := t0.Sub(start)

	if err != nil {
		logger.Errorf("ProcessProposal error[%v]", err)
		return nil, err
	}

	logger.Infof("Invoke Ok, ProcessProposal duration=%v,rsp=%v,%s", duration, rsp, unit.Payload)

	return unit, nil
}

func StopByName(contractid []byte, chainID string, txid string, ccName string, ccPath string, ccVersion string, deleteImage bool) error {
	usrcc := &ucc.UserChaincode{
		Name:    ccName,
		Path:    ccPath,
		Version: ccVersion,
		Enabled: true,
	}
	err := ucc.StopUserCC(contractid, chainID, usrcc, txid, deleteImage)
	if err != nil {
		errMsg := fmt.Sprintf("StopUserCC err[%s]-[%s]-err[%s]", chainID, ccName, err)
		return errors.New(errMsg)
	}

	return nil
}

func Stop(contractid []byte, chainID string, deployId []byte, txid string, deleteImage bool) error {
	logger.Infof("==========Stop enter=======")
	logger.Infof("deployId[%s]txid[%s]", hex.EncodeToString(deployId), txid)
	defer logger.Infof("-----------Stop exit--------")

	setChainId := "palletone"
	if chainID != "" {
		setChainId = chainID
	}
	if txid == "" {
		return errors.New("input param txid is nil")
	}
	cc, err := cclist.GetChaincode(chainID, deployId)
	if err != nil {
		return err
	}
	err = StopByName(contractid, setChainId, txid, cc.Name, cc.Path, cc.Version, deleteImage)
	if err == nil {
		cclist.DelChaincode(chainID, cc.Name, cc.Version)
	}

	return err
}

//func peerContractMockConfigInit() {
//	viper.Set("peer.fileSystemPath", "./chaincodes")
//	viper.Set("peer.address", "127.0.0.1:12345")
//	viper.Set("chaincode.executetimeout", 20*time.Second)
//
//	viper.Set("vm.endpoint", "unix:///var/run/docker.sock")
//	viper.Set("chaincode.builder", "palletimg")
//
//	viper.Set("chaincode.system", map[string]string{"sample_syscc": "true"})
//}
