package errorHandle

import (
	"fmt"
	"github.com/pkg/errors"
	"os"
)

func ExitWithError(err error) {
	if err != nil {
		fmt.Printf("%v\n", errors.Cause(err))
		os.Exit(1)
	}
}
