package libreta

// Tabella codice ISO 3166-1 alpha-3 -> aggettivi di nazionalita' in inglese, come si stampano nel campo
// "Nacionalidad / Nationality" (il primo e' quello principale; gli altri sono varianti ammesse). Dato generico,
// nessun dato personale. Il codice a tre lettere dell'MRZ e' il riferimento: se la lettura OCR e' vicina
// all'aggettivo atteso per quel codice (vizExpectedNationality) si usa quello.

import "sort"

var vizNationalities = map[string][]string{
	"AFG": {"AFGHAN"}, "ALB": {"ALBANIAN"}, "DZA": {"ALGERIAN"}, "AND": {"ANDORRAN"}, "AGO": {"ANGOLAN"},
	"ATG": {"ANTIGUAN", "BARBUDAN"}, "ARG": {"ARGENTINE", "ARGENTINIAN"}, "ARM": {"ARMENIAN"}, "AUS": {"AUSTRALIAN"},
	"AUT": {"AUSTRIAN"}, "AZE": {"AZERBAIJANI"}, "BHS": {"BAHAMIAN"}, "BHR": {"BAHRAINI"}, "BGD": {"BANGLADESHI"},
	"BRB": {"BARBADIAN"}, "BLR": {"BELARUSIAN"}, "BEL": {"BELGIAN"}, "BLZ": {"BELIZEAN"}, "BEN": {"BENINESE"},
	"BTN": {"BHUTANESE"}, "BOL": {"BOLIVIAN"}, "BIH": {"BOSNIAN"}, "BWA": {"BOTSWANAN"}, "BRA": {"BRAZILIAN"},
	"BRN": {"BRUNEIAN"}, "BGR": {"BULGARIAN"}, "BFA": {"BURKINABE"}, "BDI": {"BURUNDIAN"}, "CPV": {"CAPE VERDEAN"},
	"KHM": {"CAMBODIAN"}, "CMR": {"CAMEROONIAN"}, "CAN": {"CANADIAN"}, "CAF": {"CENTRAL AFRICAN"}, "TCD": {"CHADIAN"},
	"CHL": {"CHILEAN"}, "CHN": {"CHINESE"}, "COL": {"COLOMBIAN"}, "COM": {"COMORAN"}, "COG": {"CONGOLESE"},
	"COD": {"CONGOLESE"}, "CRI": {"COSTA RICAN"}, "CIV": {"IVORIAN"}, "HRV": {"CROATIAN"}, "CUB": {"CUBAN"},
	"CYP": {"CYPRIOT"}, "CZE": {"CZECH"}, "DNK": {"DANISH"}, "DJI": {"DJIBOUTIAN"}, "DMA": {"DOMINICAN"},
	"DOM": {"DOMINICAN"}, "ECU": {"ECUADORIAN"}, "EGY": {"EGYPTIAN"}, "SLV": {"SALVADORAN"}, "GNQ": {"EQUATORIAL GUINEAN"},
	"ERI": {"ERITREAN"}, "EST": {"ESTONIAN"}, "SWZ": {"SWAZI"}, "ETH": {"ETHIOPIAN"}, "FJI": {"FIJIAN"},
	"FIN": {"FINNISH"}, "FRA": {"FRENCH"}, "GAB": {"GABONESE"}, "GMB": {"GAMBIAN"}, "GEO": {"GEORGIAN"},
	"DEU": {"GERMAN"}, "GHA": {"GHANAIAN"}, "GRC": {"GREEK"}, "GRD": {"GRENADIAN"}, "GTM": {"GUATEMALAN"},
	"GIN": {"GUINEAN"}, "GNB": {"BISSAU-GUINEAN"}, "GUY": {"GUYANESE"}, "HTI": {"HAITIAN"}, "HND": {"HONDURAN"},
	"HKG": {"HONG KONG"}, "HUN": {"HUNGARIAN"}, "ISL": {"ICELANDIC"}, "IND": {"INDIAN"}, "IDN": {"INDONESIAN"},
	"IRN": {"IRANIAN"}, "IRQ": {"IRAQI"}, "IRL": {"IRISH"}, "ISR": {"ISRAELI"}, "ITA": {"ITALIAN"},
	"JAM": {"JAMAICAN"}, "JPN": {"JAPANESE"}, "JOR": {"JORDANIAN"}, "KAZ": {"KAZAKH", "KAZAKHSTANI"}, "KEN": {"KENYAN"},
	"KIR": {"I-KIRIBATI"}, "PRK": {"NORTH KOREAN"}, "KOR": {"KOREAN", "SOUTH KOREAN"}, "XKX": {"KOSOVAN"}, "KWT": {"KUWAITI"},
	"KGZ": {"KYRGYZ"}, "LAO": {"LAO", "LAOTIAN"}, "LVA": {"LATVIAN"}, "LBN": {"LEBANESE"}, "LSO": {"BASOTHO"},
	"LBR": {"LIBERIAN"}, "LBY": {"LIBYAN"}, "LIE": {"LIECHTENSTEINER"}, "LTU": {"LITHUANIAN"}, "LUX": {"LUXEMBOURGER", "LUXEMBOURGISH"},
	"MAC": {"MACANESE"}, "MDG": {"MALAGASY"}, "MWI": {"MALAWIAN"}, "MYS": {"MALAYSIAN"}, "MDV": {"MALDIVIAN"},
	"MLI": {"MALIAN"}, "MLT": {"MALTESE"}, "MHL": {"MARSHALLESE"}, "MRT": {"MAURITANIAN"}, "MUS": {"MAURITIAN"},
	"MEX": {"MEXICAN"}, "FSM": {"MICRONESIAN"}, "MDA": {"MOLDOVAN"}, "MCO": {"MONEGASQUE"}, "MNG": {"MONGOLIAN"},
	"MNE": {"MONTENEGRIN"}, "MAR": {"MOROCCAN"}, "MOZ": {"MOZAMBICAN"}, "MMR": {"MYANMAR", "BURMESE"}, "NAM": {"NAMIBIAN"},
	"NRU": {"NAURUAN"}, "NPL": {"NEPALESE", "NEPALI"}, "NLD": {"DUTCH"}, "NZL": {"NEW ZEALAND", "NEW ZEALANDER"},
	"NIC": {"NICARAGUAN"}, "NER": {"NIGERIEN"}, "NGA": {"NIGERIAN"}, "MKD": {"MACEDONIAN", "NORTH MACEDONIAN"},
	"NOR": {"NORWEGIAN"}, "OMN": {"OMANI"}, "PAK": {"PAKISTANI"}, "PLW": {"PALAUAN"}, "PSE": {"PALESTINIAN"},
	"PAN": {"PANAMANIAN"}, "PNG": {"PAPUA NEW GUINEAN"}, "PRY": {"PARAGUAYAN"}, "PER": {"PERUVIAN"},
	"PHL": {"FILIPINO", "PHILIPPINE"}, "POL": {"POLISH"}, "PRT": {"PORTUGUESE"}, "QAT": {"QATARI"}, "ROU": {"ROMANIAN"},
	"RUS": {"RUSSIAN"}, "RWA": {"RWANDAN"}, "KNA": {"KITTITIAN", "NEVISIAN"}, "LCA": {"SAINT LUCIAN"},
	"VCT": {"VINCENTIAN"}, "WSM": {"SAMOAN"}, "SMR": {"SAMMARINESE"}, "STP": {"SAO TOMEAN"}, "SAU": {"SAUDI", "SAUDI ARABIAN"},
	"SEN": {"SENEGALESE"}, "SRB": {"SERBIAN"}, "SYC": {"SEYCHELLOIS"}, "SLE": {"SIERRA LEONEAN"}, "SGP": {"SINGAPOREAN"},
	"SVK": {"SLOVAK"}, "SVN": {"SLOVENIAN", "SLOVENE"}, "SLB": {"SOLOMON ISLANDER"}, "SOM": {"SOMALI"}, "ZAF": {"SOUTH AFRICAN"},
	"SSD": {"SOUTH SUDANESE"}, "ESP": {"SPANISH"}, "LKA": {"SRI LANKAN"}, "SDN": {"SUDANESE"}, "SUR": {"SURINAMESE"},
	"SWE": {"SWEDISH"}, "CHE": {"SWISS"}, "SYR": {"SYRIAN"}, "TWN": {"TAIWANESE"}, "TJK": {"TAJIK"}, "TZA": {"TANZANIAN"},
	"THA": {"THAI"}, "TLS": {"TIMORESE"}, "TGO": {"TOGOLESE"}, "TON": {"TONGAN"}, "TTO": {"TRINIDADIAN", "TOBAGONIAN"},
	"TUN": {"TUNISIAN"}, "TUR": {"TURKISH"}, "TKM": {"TURKMEN"}, "TUV": {"TUVALUAN"}, "UGA": {"UGANDAN"},
	"UKR": {"UKRAINIAN"}, "ARE": {"EMIRATI"}, "GBR": {"BRITISH"}, "USA": {"AMERICAN"}, "URY": {"URUGUAYAN"},
	"UZB": {"UZBEK"}, "VUT": {"VANUATUAN"}, "VAT": {"VATICAN"}, "VEN": {"VENEZUELAN"}, "VNM": {"VIETNAMESE"},
	"YEM": {"YEMENI"}, "ZMB": {"ZAMBIAN"}, "ZWE": {"ZIMBABWEAN"},
}

// vizDemonyms: aggettivo -> codice ISO, derivato dalla tabella (a parita' di aggettivo vince il codice minore).
var vizDemonyms = func() map[string]string {
	codes := make([]string, 0, len(vizNationalities))
	for c := range vizNationalities {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	m := map[string]string{}
	for i := len(codes) - 1; i >= 0; i-- {
		for _, a := range vizNationalities[codes[i]] {
			m[a] = codes[i]
		}
	}
	return m
}()

// vizExpectedNationality sceglie, tra le letture OCR dell'aggettivo (ordinate per priorita'), quella piu' vicina a un
// aggettivo valido per il codice ISO dell'MRZ e restituisce l'aggettivo atteso. ok=false se il codice e' sconosciuto
// o nessuna lettura somiglia (similarita' < vizNatSim) a un aggettivo atteso: il testo letto resta com'e'.
func vizExpectedNationality(code string, reads []string) (string, bool) {
	adjs := vizNationalities[code]
	best, bestS := "", 0.0
	for _, a := range adjs {
		for _, r := range reads {
			if s := vizSim(norm(r), norm(a)); s > bestS+1e-9 {
				best, bestS = a, s
			}
		}
	}
	if bestS >= vizNatSim {
		return best, true
	}
	return "", false
}

// vizNatSim: somiglianza minima tra la lettura e l'aggettivo atteso per il codice MRZ. Bassa perche' il codice e' un
// riferimento forte (un aggettivo di 9 lettere con 4 sbagliate vale 0,56).
const vizNatSim = 0.5
