package xlsx

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

// Calculation-chain sheet IDs carry forward to following entries. If an entry
// that switches sheets is removed, its surviving successor needs an explicit ID.
func clearScopeCalculationChain(files map[string][]byte, sheet string, cleared map[string]bool) error {
	rels, err := parseScopeXML(files["xl/_rels/workbook.xml.rels"])
	if err != nil {
		return err
	}
	relRoot := rels.child("Relationships")
	chainPath := "xl/calcChain.xml"
	var chainRel *scopeNode
	for _, rel := range relRoot.children("Relationship") {
		if strings.HasSuffix(rel.attrs["Type"], "/calcChain") {
			chainRel = rel
			chainPath = strings.TrimPrefix(rel.attrs["Target"], "/")
			if !strings.HasPrefix(chainPath, "xl/") {
				chainPath = path.Join("xl", chainPath)
			}
			break
		}
	}
	chainXML, exists := files[chainPath]
	if !exists {
		return nil
	}
	workbook, err := parseScopeXML(files["xl/workbook.xml"])
	if err != nil {
		return err
	}
	sheetID := ""
	for _, node := range workbook.child("workbook").child("sheets").children("sheet") {
		if node.attrs["name"] == sheet {
			sheetID = node.attrs["sheetId"]
			break
		}
	}
	if _, err := strconv.Atoi(sheetID); err != nil {
		return fmt.Errorf("cannot resolve calculation-chain sheet %q", sheet)
	}
	chain, err := parseScopeXML(chainXML)
	if err != nil {
		return err
	}
	chainRoot := chain.child("calcChain")
	if chainRoot == nil {
		return fmt.Errorf("invalid calculation-chain XML")
	}
	currentSheet, emittedSheet := "", ""
	changed, remaining := false, 0
	for _, cell := range chainRoot.children("c") {
		if id := cell.attrs["i"]; id != "" {
			currentSheet = id
		}
		if currentSheet == sheetID && cleared[cell.attrs["r"]] {
			cell.remove = true
			changed = true
			continue
		}
		if currentSheet != emittedSheet && cell.attrs["i"] == "" {
			cell.set("i", currentSheet)
		}
		emittedSheet = currentSheet
		remaining++
	}
	if !changed {
		return nil
	}
	if remaining > 0 {
		files[chainPath] = chain.bytes()
		return nil
	}
	delete(files, chainPath)
	if chainRel != nil {
		chainRel.remove = true
		files["xl/_rels/workbook.xml.rels"] = rels.bytes()
	}
	types, err := parseScopeXML(files["[Content_Types].xml"])
	if err != nil {
		return err
	}
	for _, node := range types.child("Types").children("Override") {
		if strings.TrimPrefix(node.attrs["PartName"], "/") == chainPath {
			node.remove = true
		}
	}
	files["[Content_Types].xml"] = types.bytes()
	return nil
}
