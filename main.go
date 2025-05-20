package main

import (
	"fmt"
	"parsing_media/gazeta"
	"parsing_media/lenta"
	"parsing_media/ria"
)

func main() {
	var num string

	fmt.Print("1 - RIA\n2 - Gazeta\n3 - Lenta\n\n")
	fmt.Scan(&num)

	switch num {

	case "1":
		ria.RiaMain()

	case "2":
		gazeta.GazetaMain()

	case "3":
		lenta.LentaMain()
	}
}
