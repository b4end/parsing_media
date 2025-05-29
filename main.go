package main

import (
	"fmt"
	"parsing_media/parsers"
)

func main() {
	var num string

	fmt.Print("1 - RIA\n2 - Gazeta\n3 - Lenta\n4 - Vesti\n5 - Kommersant\n6 - MK\n7 - Fontanka\n8 - Smotrim\n9 - Banki\n10 - DumaTV\n\n")
	fmt.Scan(&num)

	switch num {

	case "1":
		parsers.RiaMain()

	case "2":
		parsers.GazetaMain()

	case "3":
		parsers.LentaMain()

	case "4":
		parsers.VestiMain()

	case "5":
		parsers.KommersMain()

	case "6":
		parsers.MKMain()

	case "7":
		parsers.FontankaMain()

	case "8":
		parsers.SmotrimMain()

	case "9":
		parsers.BankiMain()

	case "10":
		parsers.DumaTVMain()
	}

}
