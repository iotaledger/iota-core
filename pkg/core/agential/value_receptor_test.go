package agential

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValueReceptor(t *testing.T) {
	myInt := NewValueReceptor[int]()

	var wg sync.WaitGroup
	wg.Add(2)

	myInt.OnUpdate(func(prevValue, newValue int) {
		if prevValue != 0 {
			require.Equal(t, prevValue+1, newValue)
		}
		//fmt.Println("0> [START] OnUpdate", prevValue, newValue)
		defer fmt.Println("0> [DONE] OnUpdate ", prevValue, newValue)

		time.Sleep(200 * time.Millisecond)

		if newValue == 3 {
			wg.Done()
		}
	})

	go func() {
		time.Sleep(50 * time.Millisecond)

		myInt.OnUpdate(func(prevValue, newValue int) {
			if prevValue != 0 {
				require.Equal(t, prevValue+1, newValue)
			}

			defer fmt.Println("1> [DONE] OnUpdate ", prevValue, newValue)

			time.Sleep(400 * time.Millisecond)

			if newValue == 3 {
				wg.Done()
			}
		})
	}()

	myInt.Set(1)
	myInt.Set(2)
	myInt.Set(3)

	wg.Wait()
}
