package cli

import (
	"fmt"
)

// report prints the outcome of an operation that only reports failure.
func report(op string, err error) {
	if err != nil {
		fmt.Printf("%s failed: %v\n", op, err)
	}
}

func printAddrList(addrs []uintptr, emptyMsg string) {
	if len(addrs) == 0 {
		fmt.Println(emptyMsg)
		return
	}
	for _, addr := range addrs {
		fmt.Printf("  0x%x\n", addr)
	}
}
