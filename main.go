package main

import (
	"fmt"
	"parsing_media/gazeta"
	"parsing_media/kommers"
	"parsing_media/lenta"
	"parsing_media/ria"
	"parsing_media/vesti"
)

func main() {
	var num string

	fmt.Print("1 - RIA\n2 - Gazeta\n3 - Lenta\n4 - Vesti\n5 - Kommersant\n\n")
	fmt.Scan(&num)

	switch num {

	case "1":
		ria.RiaMain()

	case "2":
		gazeta.GazetaMain()

	case "3":
		lenta.LentaMain()

	case "4":
		vesti.VestiMain()

	case "5":
		kommers.KommersMain()
	}
}
