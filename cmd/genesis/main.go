package main

import (
	"flag"
	"fmt"
	"math/big"
	"os"

	"github.com/Fantom-foundation/lachesis-base/hash"
	"github.com/Fantom-foundation/lachesis-base/inter/idx"
	"github.com/Fantom-foundation/lachesis-base/inter/pos"
	"github.com/Fantom-foundation/lachesis-base/kvdb/memorydb"
	"github.com/Fantom-foundation/lachesis-base/lachesis"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/Fantom-foundation/go-opera/evmcore"
	"github.com/Fantom-foundation/go-opera/integration/makefakegenesis"
	"github.com/Fantom-foundation/go-opera/integration/makegenesis"
	"github.com/Fantom-foundation/go-opera/inter/drivertype"
	"github.com/Fantom-foundation/go-opera/inter/iblockproc"
	"github.com/Fantom-foundation/go-opera/inter/ier"
	"github.com/Fantom-foundation/go-opera/inter/validatorpk"
	"github.com/Fantom-foundation/go-opera/opera"
	"github.com/Fantom-foundation/go-opera/opera/contracts/driver"
	"github.com/Fantom-foundation/go-opera/opera/contracts/driver/drivercall"
	"github.com/Fantom-foundation/go-opera/opera/contracts/driverauth"
	"github.com/Fantom-foundation/go-opera/opera/contracts/evmwriter"
	"github.com/Fantom-foundation/go-opera/opera/contracts/netinit"
	"github.com/Fantom-foundation/go-opera/opera/contracts/sfc"
	"github.com/Fantom-foundation/go-opera/opera/genesis"
	"github.com/Fantom-foundation/go-opera/opera/genesis/gpos"
	"github.com/Fantom-foundation/go-opera/opera/genesisstore"
	futils "github.com/Fantom-foundation/go-opera/utils"
	"github.com/Fantom-foundation/go-opera/utils/iodb"
	"github.com/Fantom-foundation/go-opera/valkeystore"
)

func main() {
	num := flag.Int("num", 3, "number of validators")
	balanceParam := flag.Int("balance", 1000000000, "validators balance")
	stakeParam := flag.Int("stake", 5000000, "stake amount")
	flag.Parse()

	fmt.Println("Validator num: ", *num)

	rules := opera.OninoTestNetRules()
	balance := futils.ToFtm(uint64(*balanceParam))
	stake := futils.ToFtm(uint64(*stakeParam))
	epoch := idx.Epoch(2)
	block := idx.Block(1)

	validators := make(gpos.Validators, 0, *num)
	for i := 1; i <= *num; i++ {
		nodeKey, err := crypto.GenerateKey()
		if err != nil {
			utils.Fatalf("Failed to generate node key: %v", err)
		}
		fmt.Printf("Node Private key %d: %x\n", i, crypto.FromECDSA(nodeKey))
		fmt.Printf("Node Public key %d: %x\n", i, crypto.FromECDSAPub(&nodeKey.PublicKey))

		valKeystore := valkeystore.NewDefaultFileRawKeystore(".")
		key, err := crypto.GenerateKey()
		if err != nil {
			utils.Fatalf("Failed to generate key: %v", err)
		}

		pubkeyraw := crypto.FromECDSAPub(&key.PublicKey)
		valPublicKey := validatorpk.PubKey{
			Raw:  pubkeyraw,
			Type: validatorpk.Types.Secp256k1,
		}
		err = valKeystore.Add(valPublicKey, crypto.FromECDSA(key), "123")
		if err != nil {
			utils.Fatalf("Failed to create account: %v", err)
		}

		fmt.Printf("Validator Private key %d: %x\n", i, crypto.FromECDSA(key))
		fmt.Printf("ValidatorPublic key %d: %x\n", i, valPublicKey.Bytes())
		fmt.Printf("Address %d: %x\n", i, crypto.PubkeyToAddress(key.PublicKey))

		addr := crypto.PubkeyToAddress(key.PublicKey)
		validators = append(validators, gpos.Validator{
			ID:      idx.ValidatorID(i),
			Address: addr,
			PubKey: validatorpk.PubKey{
				Raw:  pubkeyraw,
				Type: validatorpk.Types.Secp256k1,
			},
			CreationTime:     makefakegenesis.FakeGenesisTime,
			CreationEpoch:    0,
			DeactivatedTime:  0,
			DeactivatedEpoch: 0,
			Status:           0,
		})
	}

	builder := makegenesis.NewGenesisBuilder(memorydb.NewProducer(""))

	// add balances to validators
	var delegations []drivercall.Delegation
	for _, val := range validators {
		builder.AddBalance(val.Address, balance)
		delegations = append(delegations, drivercall.Delegation{
			Address:            val.Address,
			ValidatorID:        val.ID,
			Stake:              stake,
			LockedStake:        new(big.Int),
			LockupFromEpoch:    0,
			LockupEndTime:      0,
			LockupDuration:     0,
			EarlyUnlockPenalty: new(big.Int),
			Rewards:            new(big.Int),
		})
	}

	// deploy essential contracts
	// pre deploy NetworkInitializer
	builder.SetCode(netinit.ContractAddress, netinit.GetContractBin())
	// pre deploy NodeDriver
	builder.SetCode(driver.ContractAddress, driver.GetContractBin())
	// pre deploy NodeDriverAuth
	builder.SetCode(driverauth.ContractAddress, driverauth.GetContractBin())
	// pre deploy SFC
	builder.SetCode(sfc.ContractAddress, sfc.GetContractBin())
	// set non-zero code for pre-compiled contracts
	builder.SetCode(evmwriter.ContractAddress, []byte{0})

	builder.SetCurrentEpoch(ier.LlrIdxFullEpochRecord{
		LlrFullEpochRecord: ier.LlrFullEpochRecord{
			BlockState: iblockproc.BlockState{
				LastBlock: iblockproc.BlockCtx{
					Idx:     block - 1,
					Time:    evmcore.FakeGenesisTime,
					Atropos: hash.Event{},
				},
				FinalizedStateRoot:    hash.Hash{},
				EpochGas:              0,
				EpochCheaters:         lachesis.Cheaters{},
				CheatersWritten:       0,
				ValidatorStates:       make([]iblockproc.ValidatorBlockState, 0),
				NextValidatorProfiles: make(map[idx.ValidatorID]drivertype.Validator),
				DirtyRules:            nil,
				AdvanceEpochs:         0,
			},
			EpochState: iblockproc.EpochState{
				Epoch:             epoch - 1,
				EpochStart:        evmcore.FakeGenesisTime,
				PrevEpochStart:    evmcore.FakeGenesisTime - 1,
				EpochStateRoot:    hash.Zero,
				Validators:        pos.NewBuilder().Build(),
				ValidatorStates:   make([]iblockproc.ValidatorEpochState, 0),
				ValidatorProfiles: make(map[idx.ValidatorID]drivertype.Validator),
				Rules:             rules,
			},
		},
		Idx: epoch - 1,
	})

	var owner common.Address
	if *num != 0 {
		owner = validators[0].Address
	}

	blockProc := makegenesis.DefaultBlockProc()
	genesisTxs := makefakegenesis.GetGenesisTxs(epoch-2, validators, builder.TotalSupply(), delegations, owner)
	err := builder.ExecuteGenesisTxs(blockProc, genesisTxs)
	if err != nil {
		panic(err)
	}

	header := genesis.Header{
		GenesisID:   builder.CurrentHash(),
		NetworkID:   rules.NetworkID,
		NetworkName: rules.Name,
	}
	//genesisStore := builder.Build(header)

	//genesisstore.SaveGenesisStore(genesisStore, nil)

	//fmt.Printf("Genesis: %x\n", genesisStore.Genesis())

	f, err := os.OpenFile("genesis.g", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	tmpDir, err := os.MkdirTemp("", "opera-genesis")
	if err != nil {
		panic(err)
	}
	//defer os.RemoveAll(tmpDirBlocks)

	// write blocks
	writer := newUnitWriter(f)
	err = writer.Start(header, genesisstore.BlocksSection(0), tmpDir)
	if err != nil {
		panic(err)
	}
	//buf := &memFile{bytes.NewBuffer(nil)}
	for i := len(builder.Blocks()) - 1; i >= 0; i-- {
		_ = rlp.Encode(writer, builder.Blocks()[i])
	}

	blocksHash, err := writer.Flush()
	if err != nil {
		panic(err)
	}
	fmt.Printf("- Blocks hash: %v \n", blocksHash.String())

	// write epochs
	writer = newUnitWriter(f)
	err = writer.Start(header, genesisstore.EpochsSection(0), tmpDir)
	if err != nil {
		panic(err)
	}
	for i := len(builder.Epochs()) - 1; i >= 0; i-- {
		_ = rlp.Encode(writer, builder.Epochs()[i])
	}
	epochsHash, err := writer.Flush()
	if err != nil {
		panic(err)
	}
	fmt.Printf("- Epochs hash: %v \n", epochsHash.String())

	// write EVM
	writer = newUnitWriter(f)
	err = writer.Start(header, genesisstore.EvmSection(0), tmpDir)
	if err != nil {
		panic(err)
	}
	it := builder.EvmStore().EvmDb.NewIterator(nil, nil)
	defer it.Release()
	_ = iodb.Write(writer, it)
	evmHash, err := writer.Flush()
	if err != nil {
		panic(err)
	}
	fmt.Printf("- EVM hash: %v \n", evmHash.String())
	fmt.Printf("Genesis ID: %s \n", header.GenesisID.String())
}
