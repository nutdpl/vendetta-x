package dragon

import "math/rand"

// Daily allowances.
const (
	DailyForestFights = 15
	DailyPlayerFights = 3
	MaxLevel          = 12 // beating the level-12 master earns the Red Dragon fight
)

// Item is a buyable weapon or armor tier: a name, its power, and its price.
type Item struct {
	Name  string
	Power int
	Price int
}

// Weapons and Armors are the shop tiers, index 0 = starting gear (free).
var Weapons = []Item{
	{"Fists", 0, 0},
	{"Stick", 5, 200},
	{"Dagger", 10, 1000},
	{"Short Sword", 20, 3000},
	{"Long Sword", 30, 10000},
	{"Huge Axe", 40, 30000},
	{"Bone Cruncher", 60, 100000},
	{"Twin Swords", 80, 150000},
	{"Power Axe", 120, 200000},
	{"Able's Sword", 180, 400000},
	{"Wan's Weapon", 280, 1000000},
	{"Spear of Gold", 400, 4000000},
	{"Death Sword", 600, 10000000},
}

var Armors = []Item{
	{"Coat", 0, 0},
	{"Heavy Coat", 3, 200},
	{"Leather Vest", 10, 1000},
	{"Bronze Armor", 20, 3000},
	{"Chain Mail", 30, 10000},
	{"Plate Mail", 50, 30000},
	{"Ice Armor", 75, 100000},
	{"Graphite Armor", 100, 150000},
	{"Erfo's Armor", 150, 200000},
	{"Black Knight", 220, 400000},
	{"Magic Armor", 350, 1000000},
	{"Gowns of Power", 500, 4000000},
	{"Dragon Armor", 700, 10000000},
}

// WeaponPower / ArmorPower clamp an index into the tier tables.
func WeaponPower(n int) int { return itemPower(Weapons, n) }
func ArmorPower(n int) int  { return itemPower(Armors, n) }
func itemPower(items []Item, n int) int {
	if n < 0 {
		return 0
	}
	if n >= len(items) {
		n = len(items) - 1
	}
	return items[n].Power
}

// Master is the level guardian a warrior must beat to advance. Beating the
// master of your current level promotes you and boosts your stats.
type Master struct {
	Name      string
	ExpNeeded int // experience required before you may challenge
	HP        int
	Attack    int
	// Stat gains awarded on victory (added to the warrior's base stats).
	GainHP, GainAttack, GainDefense int
}

// Masters is indexed by the warrior's CURRENT level (1..12). Index 0 is unused.
var Masters = []Master{
	{}, // 0 unused
	{"Halder", 100, 30, 10, 15, 3, 2},
	{"Barak", 400, 70, 18, 25, 5, 4},
	{"Aragorn", 1000, 150, 30, 40, 8, 6},
	{"Olodrin", 4000, 300, 55, 60, 12, 10},
	{"Mistress Farae", 10000, 600, 90, 90, 18, 15},
	{"Gerrick", 40000, 1200, 140, 140, 26, 22},
	{"Cerebrum", 100000, 2400, 220, 220, 38, 32},
	{"Aurora", 400000, 4800, 350, 350, 55, 48},
	{"Glerart", 1000000, 9000, 550, 550, 80, 70},
	{"Mordagan", 4000000, 18000, 850, 850, 120, 105},
	{"Turgon", 10000000, 36000, 1300, 1400, 180, 160},
	{"Sandtiger", 40000000, 70000, 2000, 2200, 260, 240},
}

// Monster is a forest foe.
type Monster struct {
	Name    string
	HP      int
	Attack  int
	Exp     int
	Gold    int
	DeadMsg string // flavor shown when it falls
}

// monsterNames are flavor names rolled in the forest; combat scales with level,
// not the name, so any name can appear at any level.
var monsterNames = []struct{ name, dead string }{
	{"a Crazed Gnome", "It squeals and pops like a grape."},
	{"a Hungry Wolf", "It whimpers once and is still."},
	{"a Pit Viper", "You crush its skull underfoot."},
	{"a Goblin Raider", "It drops its rusty blade and falls."},
	{"an Evil Squirrel", "Cute, but very dead."},
	{"a Rabid Bear", "The great beast topples with a groan."},
	{"a Skeleton Warrior", "Its bones clatter to the path."},
	{"a Dark Knight", "His black armor cracks open."},
	{"a Forest Troll", "It collapses into a heap of moss."},
	{"a Wandering Ghoul", "It dissolves into foul mist."},
	{"a Were-Rat", "It shrinks back into a twitching rat."},
	{"an Ogre Mage", "Its spell dies on its lips."},
}

// RollMonster builds a forest foe scaled to the warrior's level.
func RollMonster(level int, rng *rand.Rand) Monster {
	if level < 1 {
		level = 1
	}
	pick := monsterNames[rng.Intn(len(monsterNames))]
	// Base stats grow with level; small randomness keeps fights varied.
	hp := 12*level + rng.Intn(8*level+1)
	atk := 5*level + rng.Intn(3*level+1)
	exp := 10*level + rng.Intn(10*level+1)
	gold := 8*level + rng.Intn(12*level+1)
	return Monster{Name: pick.name, HP: hp, Attack: atk, Exp: exp, Gold: gold, DeadMsg: pick.dead}
}

// Damage resolves one swing: attacker power vs defender guard, with a random
// spread, never below a token 1 on a connecting hit.
func Damage(attack, defense int, rng *rand.Rand) int {
	base := attack - defense/2
	if base < 1 {
		base = 1
	}
	// 50%..120% of base.
	d := base/2 + rng.Intn(base*7/10+1) + rng.Intn(base*3/10+1)
	if d < 1 {
		d = 1
	}
	return d
}

// CanChallengeMaster reports whether the warrior has the experience to face the
// master of their current level.
func CanChallengeMaster(c *Character) bool {
	if c.Level < 1 || c.Level > MaxLevel {
		return false
	}
	return c.Exp >= Masters[c.Level].ExpNeeded
}

// HealCost is the gold to restore one hit point at the healer (scales w/ level).
func HealCost(level int) int {
	if level < 1 {
		level = 1
	}
	return level * 5
}
