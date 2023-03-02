package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"flag"

	uc "github.com/unicorn-engine/unicorn/bindings/go/unicorn"
)

func WriteCheckpoint(ram map[uint32](uint32), fn string, step int) {
	trieroot := RamToTrie(ram)
	dat := TrieToJson(trieroot, step)
	fmt.Printf("writing %s len %d with root %s\n", fn, len(dat), trieroot)
	ioutil.WriteFile(fn, dat, 0644)
}

func main() {
	var target int
	var programPath string
	var inputHash string
	var evm bool
	var countOps bool
	var basedir string
	var outputGolden bool
	var blockNumber int
	var root string

	defaultBasedir := os.Getenv("BASEDIR")
	if len(defaultBasedir) == 0 {
		defaultBasedir = "/tmp/cannon"
	}
	flag.StringVar(&basedir, "basedir", defaultBasedir, "Directory to read inputs, write outputs, and cache preimage oracle data.")
	flag.IntVar(&blockNumber, "blockNumber", -1, "For state transition programs (e.g. rollups), used to create a separate subdirectory in the basedir for each block inputs/outputs and snapshots.")
	flag.IntVar(&target, "target", -1, "Target number of instructions to execute in the trace. If < 0 will execute until termination")
	flag.StringVar(&programPath, "program", "mipigo/minigeth.bin", "Path to binary file containing the program to run")
	flag.StringVar(&inputHash, "inputHash", "", "The input to the execution. A hex encoded hash (32 bytes, 64 chars) which will be placed in memory location 0x30000000. If none will load the file at <root>/input")
	flag.BoolVar(&countOps, "countOps", false, "Should the MIPS emulator count how many instructions it executes? This slows down execution significantly (factor of 10)")
	flag.BoolVar(&evm, "evm", false, "If the program should be executed by a MIPS emulator running inside the EVM. This is much much slower than using the Unicorn emulator but exactly replicates the fault proving environment.")
	flag.BoolVar(&outputGolden, "outputGolden", false, "Do not read any inputs and instead produce a snapshot of the state prior to execution. Written to <basedir>/golden.json")
	flag.Parse()

	if blockNumber >= 0 {
		root = fmt.Sprintf("%s/%d_%d", basedir, 0, blockNumber)
	} else {
		root = basedir
	}

	regfault := -1
	regfault_str, regfault_valid := os.LookupEnv("REGFAULT")
	if regfault_valid {
		regfault, _ = strconv.Atoi(regfault_str)
	}

	// step 1, generate the checkpoints every million steps using unicorn
	ram := make(map[uint32](uint32))

	lastStep := 1 // this could be larger than an int32..
	if evm {
		// TODO: fix this
		log.Fatal("EVM execution currently not supported")
		/*ZeroRegisters(ram)
		LoadMappedFile(programPath, ram, 0)
		WriteCheckpoint(ram, "/tmp/cannon/golden.json", -1)
		LoadMappedFile(fmt.Sprintf("%s/input", root), ram, 0x30000000)
		RunWithRam(ram, target-1, 0, root, nil)
		lastStep += target - 1
		fn := fmt.Sprintf("%s/checkpoint_%d.json", root, lastStep)
		WriteCheckpoint(ram, fn, lastStep)*/
	} else {

		instrCallback := func(step int, mu uc.Unicorn, ram map[uint32](uint32)) {
			// it seems this runs before the actual step happens
			// this can be raised to 10,000,000 if the files are too large
			//if (target == -1 && step%10000000 == 0) || step == target {
			// first run checkpointing is disabled for now since is isn't used
			if step == regfault {
				fmt.Printf("regfault at step %d\n", step)
				mu.RegWrite(uc.MIPS_REG_V0, 0xbabababa)
			}
			if step == target {
				SyncRegs(mu, ram)
				fn := fmt.Sprintf("%s/checkpoint_%d.json", root, step)
				WriteCheckpoint(ram, fn, step)
				if step == target {
					// done
					mu.RegWrite(uc.MIPS_REG_PC, 0x5ead0004)
				}
			}
			lastStep = step + 1
		}
		if !countOps { // No need for a callback if we are not counting the instructions
			instrCallback = nil
		}

		mu := GetHookedUnicorn(root, ram, instrCallback)

		ZeroRegisters(ram)
		// not ready for golden yet
		LoadMappedFileUnicorn(mu, programPath, ram, 0)
		if outputGolden {
			WriteCheckpoint(ram, fmt.Sprintf("%s/golden.json", root), -1)
			fmt.Println("Writing golden snapshot and exiting early without execution")
			os.Exit(0)
		}

		// insert the input into the golden snapshot
		if len(inputHash) > 0 {
			dat, err := hex.DecodeString(inputHash)
			check(err)
			LoadMappedDataUnicorn(mu, dat, ram, 0x30000000)
		} else {
			LoadMappedFileUnicorn(mu, fmt.Sprintf("%s/input", root), ram, 0x30000000)
		}

		mu.Start(0, 0x5ead0004)
		SyncRegs(mu, ram)
	}

	if target == -1 {
		if ram[0x30000800] != 0x1337f00d {
			log.Fatal("failed to output state root, exiting")
		}

		output_filename := fmt.Sprintf("%s/output", root)
		outputs, err := ioutil.ReadFile(output_filename)
		check(err)
		real := append([]byte{0x13, 0x37, 0xf0, 0x0d}, outputs...)

		output := []byte{}
		for i := 0; i < 0x44; i += 4 {
			t := make([]byte, 4)
			binary.BigEndian.PutUint32(t, ram[uint32(0x30000800+i)])
			output = append(output, t...)
		}

		if bytes.Compare(real, output) != 0 {
			fmt.Println("MISMATCH OUTPUT, OVERWRITING!!!")
			ioutil.WriteFile(output_filename, output[4:], 0644)
		} else {
			fmt.Println("output match")
		}

		fmt.Printf("Output: %s\n", hex.EncodeToString(output))
		if countOps {
			fmt.Printf("Number of MIPS instructions: %d\n", lastStep)
		}

		WriteCheckpoint(ram, fmt.Sprintf("%s/checkpoint_final.json", root), lastStep)

	}

	// step 2 (optional), validate each 1 million chunk in EVM

	// step 3 (super optional) validate each 1 million chunk on chain

	//RunWithRam(ram, steps, debug, nil)

}
