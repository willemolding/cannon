package main

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/fatih/color"
	uc "github.com/unicorn-engine/unicorn/bindings/go/unicorn"
)

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

var steps int = 0
var heap_start uint64 = 0

func RegRead(u *uc.Unicorn, reg int) {

}

// reimplement simple.py in go
func RunUnicorn(fn string, totalSteps int) {
	mu, err := uc.NewUnicorn(uc.ARCH_MIPS, uc.MODE_32|uc.MODE_BIG_ENDIAN)
	check(err)

	mu.HookAdd(uc.HOOK_INTR, func(mu uc.Unicorn, intno uint32) {
		if intno != 17 {
			log.Fatal("invalid interrupt ", intno, " at step ", steps)
		}
		syscall_no, _ := mu.RegRead(uc.MIPS_REG_V0)
		v0 := uint64(0)
		if syscall_no == 4020 {
			oracle_hash, _ := mu.MemRead(0x30001000, 0x20)
			hash := common.BytesToHash(oracle_hash)
			key := fmt.Sprintf("/tmp/eth/%s", hash)
			value, _ := ioutil.ReadFile(key)
			tmp := []byte{0, 0, 0, 0}
			binary.BigEndian.PutUint32(tmp, uint32(len(value)))
			mu.MemWrite(0x31000000, tmp)
			mu.MemWrite(0x31000004, value)
		} else if syscall_no == 4004 {
			fd, _ := mu.RegRead(uc.MIPS_REG_A0)
			buf, _ := mu.RegRead(uc.MIPS_REG_A1)
			count, _ := mu.RegRead(uc.MIPS_REG_A2)
			bytes, _ := mu.MemRead(buf, count)

			printer := color.New(color.FgWhite).SprintFunc()
			if fd == 1 {
				printer = color.New(color.FgGreen).SprintFunc()
			} else if fd == 2 {
				printer = color.New(color.FgRed).SprintFunc()
			}
			os.Stderr.WriteString(printer(string(bytes)))
		} else if syscall_no == 4090 {
			a0, _ := mu.RegRead(uc.MIPS_REG_A0)
			sz, _ := mu.RegRead(uc.MIPS_REG_A1)
			if a0 == 0 {
				v0 = 0x20000000 + heap_start
				heap_start += sz
			} else {
				v0 = a0
			}
		} else if syscall_no == 4045 {
			v0 = 0x40000000
		} else if syscall_no == 4120 {
			v0 = 1
		} else {
			//fmt.Println("syscall", syscall_no)
		}
		mu.RegWrite(uc.MIPS_REG_V0, v0)
		mu.RegWrite(uc.MIPS_REG_A3, 0)
	}, 0, 0)

	slowMode := true

	ram := make(map[uint32](uint32))
	if slowMode {
		mu.HookAdd(uc.HOOK_MEM_WRITE, func(mu uc.Unicorn, access int, addr uint64, size int, value int64) {
			//fmt.Printf("%X(%d) = %x\n", addr, size, value)
			// TODO: fix unaligned access
			if value == 0 {
				delete(ram, uint32(addr))
			} else {
				ram[uint32(addr)] = uint32(value)
			}
		}, 0, 0x80000000)

		ministart := time.Now()
		mu.HookAdd(uc.HOOK_CODE, func(mu uc.Unicorn, addr uint64, size uint32) {
			if steps%1000000 == 0 {
				steps_per_sec := float64(steps) * 1e9 / float64(time.Now().Sub(ministart).Nanoseconds())
				fmt.Printf("%10d pc: %x steps per s %f ram entries %d\n", steps, addr, steps_per_sec, len(ram))
			}
			steps += 1
			if totalSteps == steps {
				os.Exit(0)
			}
		}, 0, 0x80000000)
	}

	check(mu.MemMap(0, 0x80000000))

	// program
	dat, _ := ioutil.ReadFile(fn)
	mu.MemWrite(0, dat)

	// inputs
	inputFile := fmt.Sprintf("/tmp/eth/%d", 13284469)
	inputs, _ := ioutil.ReadFile(inputFile)
	mu.MemWrite(0x30000000, inputs)

	LoadMappedFile(fn, ram, 0)
	LoadMappedFile(inputFile, ram, 0x30000000)

	mu.Start(0, 0xdead0000)

}