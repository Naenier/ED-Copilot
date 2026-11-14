package trading

import "testing"

func TestRoutePowerBonusUsesSalesInOnePledgedTerritory(t *testing.T) {
	leg := func(good, power string, spread float64) M {
		return M{"key": good, "quantity": 1216, "buyPrice": 1000, "sellPrice": 1000 + spread,
			"sell": M{"controllingPower": power, "powerplayState": "Exploited"}}
	}
	outward := leg("indium", "Zemina Torval", 22642)
	inward := leg("fish", "Felicia Winters", 10877)
	route := M{"legs": []M{outward, inward}}
	AddRoutePowerBonus(route)
	if route["powerBonusPerUnit"] != float64(10189) || route["powerBonusProfit"] != float64(12389702) ||
		route["profitWithPowerBonus"] != float64(53148806) || route["powerBonusPower"] != "Zemina Torval" {
		t.Fatal(route)
	}
	inward["sell"] = M{"controllingPower": "Felicia Winters", "powerplayState": "Expansion"}
	route = M{"legs": []M{inward}}
	AddRoutePowerBonus(route)
	if route["powerBonusPower"] != nil {
		t.Fatal(route)
	}
}
