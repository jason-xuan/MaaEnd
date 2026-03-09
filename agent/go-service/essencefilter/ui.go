package essencefilter

import (
	"fmt"
	"html"
	"sort"
	"strings"

	"github.com/MaaXYZ/MaaEnd/agent/go-service/pkg/maafocus"
	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

func LogMXUHTML(ctx *maa.Context, htmlText string) {
	htmlText = strings.TrimLeft(htmlText, " \t\r\n")
	maafocus.NodeActionStarting(ctx, htmlText)
}

// LogMXUSimpleHTMLWithColor logs a simple styled span, allowing a custom color.
func LogMXUSimpleHTMLWithColor(ctx *maa.Context, text string, color string) {
	HTMLTemplate := fmt.Sprintf(`<span style="color: %s; font-weight: 500;">%%s</span>`, color)
	LogMXUHTML(ctx, fmt.Sprintf(HTMLTemplate, text))
}

// LogMXUSimpleHTML logs a simple styled span with a default color.
func LogMXUSimpleHTML(ctx *maa.Context, text string) {
	// Call the more specific function with the default color "#00bfff".
	LogMXUSimpleHTMLWithColor(ctx, text, "#00bfff")
}

// logMatchSummary - 输出“战利品 summary”，按技能组合聚合统计
func logMatchSummary(ctx *maa.Context) {
	if len(matchedCombinationSummary) == 0 {
		LogMXUSimpleHTML(ctx, "本次未锁定任何目标基质。")
		return
	}

	type viewItem struct {
		Key string
		*SkillCombinationSummary
	}

	items := make([]viewItem, 0, len(matchedCombinationSummary))
	for k, v := range matchedCombinationSummary {
		items = append(items, viewItem{Key: k, SkillCombinationSummary: v})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Key < items[j].Key
	})

	var b strings.Builder
	b.WriteString(`<div style="color: #00bfff; font-weight: 900; margin-top: 4px;">战利品摘要：</div>`)
	b.WriteString(`<table style="width: 100%; border-collapse: collapse; font-size: 12px;">`)
	b.WriteString(`<tr><th style="text-align:left; padding: 2px 4px;">武器</th><th style="text-align:left; padding: 2px 4px;">技能组合</th><th style="text-align:right; padding: 2px 4px;">锁定数量</th></tr>`)

	for _, item := range items {
		weaponText := formatWeaponNamesColoredHTML(item.Weapons)
		// 为了和前面 OCR 日志一致，summary 优先展示实际 OCR 到的技能文本
		skillSource := item.OCRSkills
		if len(skillSource) == 0 {
			// 兜底：如果没有 OCR 文本（理论上不会发生），退回到静态配置的技能中文名
			skillSource = item.SkillsChinese
		}

		formattedSkills := make([]string, len(skillSource))

		for i, s := range skillSource {
			escapedSkill := escapeHTML(s)
			formattedSkills[i] = fmt.Sprintf(`<span style="color: #064d7c;">%s</span>`, escapedSkill)
		}

		skillText := strings.Join(formattedSkills, " | ")
		b.WriteString("<tr>")
		b.WriteString(fmt.Sprintf(`<td style="padding: 2px 4px;">%s</td>`, weaponText))
		b.WriteString(fmt.Sprintf(`<td style="padding: 2px 4px;">%s</td>`, skillText))
		b.WriteString(fmt.Sprintf(`<td style="padding: 2px 4px; text-align: right;">%d</td>`, item.Count))
		b.WriteString("</tr>")
	}

	b.WriteString(`</table>`)
	LogMXUHTML(ctx, b.String())
}

// formatWeaponNamesColoredHTML - 按稀有度为每把武器着色并拼接成 HTML 片段
func formatWeaponNamesColoredHTML(weapons []WeaponData) string {
	if len(weapons) == 0 {
		return ""
	}
	var b strings.Builder
	for i, w := range weapons {
		if i > 0 {
			b.WriteString("、")
		}
		color := getColorForRarity(w.Rarity)
		b.WriteString(fmt.Sprintf(
			`<span style="color: %s;">%s</span>`,
			color, escapeHTML(w.ChineseName),
		))
	}
	return b.String()
}

func getColorForRarity(rarity int) string {
	switch rarity {
	case 6:
		return "#ff7000" // rarity 6
	case 5:
		return "#ffba03" // rarity 5
	case 4:
		return "#9451f8" // rarity 4
	case 3:
		return "#26bafb" // rarity 3
	default:
		return "#493a3a" // Default color
	}
}

// escapeHTML - 简单封装 html.EscapeString，便于后续统一替换/扩展
func escapeHTML(s string) string {
	return html.EscapeString(s)
}

// calcPlan 描述一个预刻写方案及其对目标武器的覆盖情况。
type calcPlan struct {
	slot1Names [3]string
	fixedSlot  int    // 2 = 附加属性固定, 3 = 技能属性固定
	fixedID    int    // 固定槽位的技能 ID
	fixedName  string // 固定槽位的技能中文名
	needs      []WeaponData // 未毕业目标武器中能满足的
	matched    []WeaponData // 全部目标武器中能匹配的（含已毕业）
}

// logCalculatorResult 在战利品摘要之后，按刷取地点枚举预刻写方案，
// 对每个地点输出满足未毕业需求最多的前 N 个方案。
func logCalculatorResult(ctx *maa.Context) {
	// 1. 读取选中的武器稀有度（防御性过滤，确保计算器只含选中稀有度的武器）
	opts, _ := getOptionsFromAttach(ctx, "EssenceFilterInit")
	selectedRarities := make(map[int]bool)
	if opts != nil {
		if opts.Rarity4Weapon {
			selectedRarities[4] = true
		}
		if opts.Rarity5Weapon {
			selectedRarities[5] = true
		}
		if opts.Rarity6Weapon {
			selectedRarities[6] = true
		}
	}

	// 2. 收集已毕业（本次扫描锁定）的武器名
	graduated := make(map[string]bool)
	for _, s := range matchedCombinationSummary {
		for _, w := range s.Weapons {
			graduated[w.ChineseName] = true
		}
	}

	// 3. 去重后构建目标武器列表，仅含选中稀有度，区分已毕业与未毕业
	seenTarget := make(map[string]bool)
	var allTargets []SkillCombination
	var ungraduated []SkillCombination
	for _, combo := range targetSkillCombinations {
		if len(selectedRarities) > 0 && !selectedRarities[combo.Weapon.Rarity] {
			continue
		}
		name := combo.Weapon.ChineseName
		if seenTarget[name] {
			continue
		}
		seenTarget[name] = true
		allTargets = append(allTargets, combo)
		if !graduated[name] {
			ungraduated = append(ungraduated, combo)
		}
	}

	if len(ungraduated) == 0 {
		LogMXUSimpleHTML(ctx, "所有目标武器本次均已命中，无需推荐预刻写方案。")
		return
	}

	slot1Pool := weaponDB.SkillPools.Slot1
	slot2Pool := weaponDB.SkillPools.Slot2
	slot3Pool := weaponDB.SkillPools.Slot3
	n1 := len(slot1Pool)
	const maxPlansPerLocation = 2
	fixedSlotLabel := [4]string{"", "", "附加属性", "技能属性"}

	// plansBestNeeds 返回某一 slot2/slot3 子集下的最优方案列表（按需求降序）
	enumPlans := func(availSlot2, availSlot3 []SkillPool) []calcPlan {
		var plans []calcPlan
		for i := 0; i < n1-2; i++ {
			for j := i + 1; j < n1-1; j++ {
				for k := j + 1; k < n1; k++ {
					s1Names := [3]string{slot1Pool[i].Chinese, slot1Pool[j].Chinese, slot1Pool[k].Chinese}
					s1Set := map[int]bool{
						slot1Pool[i].ID: true,
						slot1Pool[j].ID: true,
						slot1Pool[k].ID: true,
					}
					for _, s2 := range availSlot2 {
						p := calcPlan{slot1Names: s1Names, fixedSlot: 2, fixedName: s2.Chinese, fixedID: s2.ID}
						for _, combo := range allTargets {
							if s1Set[combo.SkillIDs[0]] && combo.SkillIDs[1] == s2.ID {
								p.matched = append(p.matched, combo.Weapon)
								if !graduated[combo.Weapon.ChineseName] {
									p.needs = append(p.needs, combo.Weapon)
								}
							}
						}
						if len(p.needs) > 0 {
							plans = append(plans, p)
						}
					}
					for _, s3 := range availSlot3 {
						p := calcPlan{slot1Names: s1Names, fixedSlot: 3, fixedName: s3.Chinese, fixedID: s3.ID}
						for _, combo := range allTargets {
							if s1Set[combo.SkillIDs[0]] && combo.SkillIDs[2] == s3.ID {
								p.matched = append(p.matched, combo.Weapon)
								if !graduated[combo.Weapon.ChineseName] {
									p.needs = append(p.needs, combo.Weapon)
								}
							}
						}
						if len(p.needs) > 0 {
							plans = append(plans, p)
						}
					}
				}
			}
		}
		sort.Slice(plans, func(i, j int) bool {
			if len(plans[i].needs) != len(plans[j].needs) {
				return len(plans[i].needs) > len(plans[j].needs)
			}
			return len(plans[i].matched) > len(plans[j].matched)
		})
		return plans
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		`<div style="color:#00bfff;font-weight:900;margin-top:8px;">预刻写方案推荐（%d 个未毕业需求）：</div>`,
		len(ungraduated),
	))

	if len(weaponDB.Locations) > 0 {
		// 按地点分组输出
		for _, loc := range weaponDB.Locations {
			slot2Set := make(map[int]bool)
			for _, id := range loc.Slot2IDs {
				slot2Set[id] = true
			}
			slot3Set := make(map[int]bool)
			for _, id := range loc.Slot3IDs {
				slot3Set[id] = true
			}
			var locSlot2, locSlot3 []SkillPool
			for _, s := range slot2Pool {
				if slot2Set[s.ID] {
					locSlot2 = append(locSlot2, s)
				}
			}
			for _, s := range slot3Pool {
				if slot3Set[s.ID] {
					locSlot3 = append(locSlot3, s)
				}
			}

			plans := enumPlans(locSlot2, locSlot3)
			if len(plans) == 0 {
				continue
			}

			b.WriteString(fmt.Sprintf(
				`<div style="color:#ffcc00;font-weight:900;margin-top:6px;">%s</div>`,
				escapeHTML(loc.Name),
			))
			show := maxPlansPerLocation
			if len(plans) < show {
				show = len(plans)
			}
			for idx, p := range plans[:show] {
				b.WriteString(fmt.Sprintf(
					`<div style="margin-top:3px;border-left:3px solid #ffcc00;padding-left:6px;">`+
						`<span style="color:#aaffaa;">方案 %d</span> `+
						`基础属性：<span style="color:#47b5ff;">%s</span> | `+
						`选择%s：<span style="color:#e877fe;">%s</span><br>`+
						`满足 <b>%d</b> 个需求 / 匹配 <b>%d</b> 件目标武器<br>`+
						`满足的需求：%s<br>`+
						`匹配的武器：%s</div>`,
					idx+1,
					strings.Join(p.slot1Names[:], "，"),
					fixedSlotLabel[p.fixedSlot], escapeHTML(p.fixedName),
					len(p.needs), len(p.matched),
					weaponListHTML(p.needs),
					weaponListHTML(p.matched),
				))
			}
		}
	} else {
		// 无地点数据时退化为全局列表（兜底）
		plans := enumPlans(slot2Pool, slot3Pool)
		show := 10
		if len(plans) < show {
			show = len(plans)
		}
		for idx, p := range plans[:show] {
			b.WriteString(fmt.Sprintf(
				`<div style="margin-top:5px;border-left:3px solid #00bfff;padding-left:6px;">`+
					`<span style="color:#ffcc00;font-weight:700;">方案 %d</span><br>`+
					`选择基础属性：<span style="color:#47b5ff;">%s</span><br>`+
					`选择%s：<span style="color:#e877fe;">%s</span><br>`+
					`满足 <b>%d</b> 个需求，匹配 <b>%d</b> 件目标武器<br>`+
					`满足的需求：%s<br>`+
					`匹配的武器：%s</div>`,
				idx+1,
				strings.Join(p.slot1Names[:], "，"),
				fixedSlotLabel[p.fixedSlot], escapeHTML(p.fixedName),
				len(p.needs), len(p.matched),
				weaponListHTML(p.needs),
				weaponListHTML(p.matched),
			))
		}
	}
	LogMXUHTML(ctx, b.String())
}

// weaponListHTML 将武器列表格式化为按稀有度着色的 HTML 片段。
func weaponListHTML(weapons []WeaponData) string {
	if len(weapons) == 0 {
		return "（无）"
	}
	parts := make([]string, len(weapons))
	for i, w := range weapons {
		parts[i] = fmt.Sprintf(`<span style="color:%s;">%s</span>`, getColorForRarity(w.Rarity), escapeHTML(w.ChineseName))
	}
	return strings.Join(parts, "，")
}

// formatWeaponNames - 将多把武器名格式化为展示字符串（UI 层负责拼接与本地化）
func formatWeaponNames(weapons []WeaponData) string {
	if len(weapons) == 0 {
		return ""
	}
	names := make([]string, 0, len(weapons))
	for _, w := range weapons {
		names = append(names, w.ChineseName)
	}
	// 这里采用顿号拼接，更符合中文习惯；如需本地化，可进一步抽象
	return strings.Join(names, "、")
}
