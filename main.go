package main

import (
	"fmt"
	"parsing_media/parsers/banki"
	"parsing_media/parsers/dumatv"
	"parsing_media/parsers/fontanka"
	"parsing_media/parsers/gazeta"
	"parsing_media/parsers/kommers"
	"parsing_media/parsers/lenta"
	"parsing_media/parsers/mk"
	"parsing_media/parsers/ria"
	"parsing_media/parsers/smotrim"
	"parsing_media/parsers/vesti"
)

func main() {
	var num string

	fmt.Print("1 - RIA\n2 - Gazeta\n3 - Lenta\n4 - Vesti\n5 - Kommersant\n6 - MK\n7 - Fontanka\n8 - Smotrim\n9 - Banki\n10 - DumaTV\n\n")
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

	case "6":
		mk.MKMain()

	case "7":
		fontanka.FontankaMain()

	case "8":
		smotrim.SmotrimMain()

	case "9":
		banki.BankiMain()

	case "10":
		dumatv.DumaTVMain()
	}

}
