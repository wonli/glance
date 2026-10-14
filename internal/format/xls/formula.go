package xls

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/xuri/excelize/v2"
)

// FormulaResolver maps workbook-level indexes used by BIFF formula tokens.
type FormulaResolver interface {
	SheetName(externSheetIndex uint16) (string, error)
	DefinedName(nameIndex uint16) (string, error)
}

type formulaDecoder struct {
	tokens     []byte
	position   int
	baseCell   string
	baseRow    int
	baseColumn int
	resolver   FormulaResolver
	sharedBy   *cellDelta
}

type cellDelta struct {
	rows    int
	columns int
}

// decodeBIFFFormula converts a BIFF8 RPN token stream into an Excel formula.
// Tokens which cannot be represented safely fail with an explicit error.
func decodeBIFFFormula(tokens []byte, baseCell string, resolver FormulaResolver) (string, error) {
	return decodeBIFFFormulaWithDelta(tokens, baseCell, resolver, nil)
}

func decodeBIFFFormulaWithDelta(tokens []byte, baseCell string, resolver FormulaResolver, sharedBy *cellDelta) (string, error) {
	column, row, err := excelize.CellNameToCoordinates(baseCell)
	if err != nil {
		return "", fmt.Errorf("invalid formula base cell %q: %w", baseCell, err)
	}
	decoder := formulaDecoder{
		tokens:     tokens,
		baseCell:   baseCell,
		baseRow:    row - 1,
		baseColumn: column - 1,
		resolver:   resolver,
		sharedBy:   sharedBy,
	}
	stack, err := decoder.decode()
	if err != nil {
		return "", err
	}
	if len(stack) != 1 {
		return "", fmt.Errorf("formula at %s leaves %d values on the BIFF token stack", baseCell, len(stack))
	}
	return stack[0], nil
}

// decodeBIFFSharedFormula expands references in one SHRFMLA token stream from
// the master cell to a dependent cell, respecting each reference's relative flags.
func decodeBIFFSharedFormula(tokens []byte, masterCell, targetCell string, resolver FormulaResolver) (string, error) {
	masterColumn, masterRow, err := excelize.CellNameToCoordinates(masterCell)
	if err != nil {
		return "", fmt.Errorf("invalid shared formula master cell %q: %w", masterCell, err)
	}
	targetColumn, targetRow, err := excelize.CellNameToCoordinates(targetCell)
	if err != nil {
		return "", fmt.Errorf("invalid shared formula target cell %q: %w", targetCell, err)
	}
	delta := &cellDelta{rows: targetRow - masterRow, columns: targetColumn - masterColumn}
	return decodeBIFFFormulaWithDelta(tokens, targetCell, resolver, delta)
}

// decodeBIFFArrayFormula decodes the shared expression stored on an ARRAY
// record. The caller retains ref separately when creating the XLSX array formula.
func decodeBIFFArrayFormula(tokens []byte, baseCell, ref string, resolver FormulaResolver) (string, error) {
	baseColumn, baseRow, err := excelize.CellNameToCoordinates(baseCell)
	if err != nil {
		return "", fmt.Errorf("invalid array formula anchor %q: %w", baseCell, err)
	}
	parts := strings.Split(ref, ":")
	if len(parts) > 2 {
		return "", fmt.Errorf("invalid array formula range %q", ref)
	}
	firstColumn, firstRow, err := excelize.CellNameToCoordinates(parts[0])
	if err != nil {
		return "", fmt.Errorf("invalid array formula range %q: %w", ref, err)
	}
	last := parts[len(parts)-1]
	lastColumn, lastRow, err := excelize.CellNameToCoordinates(last)
	if err != nil {
		return "", fmt.Errorf("invalid array formula range %q: %w", ref, err)
	}
	if firstColumn > lastColumn || firstRow > lastRow || firstColumn != baseColumn || firstRow != baseRow {
		return "", fmt.Errorf("array formula anchor %s does not match range %s", baseCell, ref)
	}
	return decodeBIFFFormula(tokens, baseCell, resolver)
}

func (d *formulaDecoder) decode() ([]string, error) {
	stack := make([]string, 0, 8)
	for d.position < len(d.tokens) {
		start := d.position
		raw := d.tokens[d.position]
		d.position++
		token := normalizeFormulaToken(raw)
		fail := func(err error) ([]string, error) {
			return nil, fmt.Errorf("BIFF formula token 0x%02X at byte %d: %w", raw, start, err)
		}
		take := func(size int) ([]byte, error) {
			if size < 0 || size > len(d.tokens)-d.position {
				return nil, fmt.Errorf("truncated token payload: need %d bytes, have %d", size, len(d.tokens)-d.position)
			}
			payload := d.tokens[d.position : d.position+size]
			d.position += size
			return payload, nil
		}
		push := func(value string) { stack = append(stack, value) }
		pop := func() (string, error) {
			if len(stack) == 0 {
				return "", fmt.Errorf("operator has no operand")
			}
			value := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			return value, nil
		}
		binaryOp := func(operator string) error {
			right, err := pop()
			if err != nil {
				return err
			}
			left, err := pop()
			if err != nil {
				return err
			}
			push("(" + left + operator + right + ")")
			return nil
		}

		switch token {
		case 0x01: // PtgExp: shared/array formula anchor reference.
			return fail(fmt.Errorf("shared/array formula anchor token requires its SHRFMLA or ARRAY record"))
		case 0x02: // PtgTbl data table.
			return fail(fmt.Errorf("data-table formulas are not supported"))
		case 0x03:
			if err := binaryOp("+"); err != nil {
				return fail(err)
			}
		case 0x04:
			if err := binaryOp("-"); err != nil {
				return fail(err)
			}
		case 0x05:
			if err := binaryOp("*"); err != nil {
				return fail(err)
			}
		case 0x06:
			if err := binaryOp("/"); err != nil {
				return fail(err)
			}
		case 0x07:
			if err := binaryOp("^"); err != nil {
				return fail(err)
			}
		case 0x08:
			if err := binaryOp("&"); err != nil {
				return fail(err)
			}
		case 0x09:
			if err := binaryOp("<"); err != nil {
				return fail(err)
			}
		case 0x0A:
			if err := binaryOp("<="); err != nil {
				return fail(err)
			}
		case 0x0B:
			if err := binaryOp("="); err != nil {
				return fail(err)
			}
		case 0x0C:
			if err := binaryOp(">="); err != nil {
				return fail(err)
			}
		case 0x0D:
			if err := binaryOp(">"); err != nil {
				return fail(err)
			}
		case 0x0E:
			if err := binaryOp("<>"); err != nil {
				return fail(err)
			}
		case 0x0F:
			if err := binaryOp(" "); err != nil {
				return fail(err)
			}
		case 0x10:
			if err := binaryOp(","); err != nil {
				return fail(err)
			}
		case 0x11:
			if err := binaryOp(":"); err != nil {
				return fail(err)
			}
		case 0x12:
			value, err := pop()
			if err != nil {
				return fail(err)
			}
			push("+(" + value + ")")
		case 0x13:
			value, err := pop()
			if err != nil {
				return fail(err)
			}
			push("-(" + value + ")")
		case 0x14:
			value, err := pop()
			if err != nil {
				return fail(err)
			}
			push("(" + value + ")%")
		case 0x15:
			value, err := pop()
			if err != nil {
				return fail(err)
			}
			push("(" + value + ")")
		case 0x16: // PtgMissArg
			push("")
		case 0x17: // PtgStr: cch, grbit, characters.
			prefix, err := take(2)
			if err != nil {
				return fail(err)
			}
			count := int(prefix[0])
			width := 1
			if prefix[1]&1 != 0 {
				width = 2
			}
			characters, err := take(count * width)
			if err != nil {
				return fail(err)
			}
			var value string
			if width == 2 {
				units := make([]uint16, count)
				for index := range units {
					units[index] = binary.LittleEndian.Uint16(characters[index*2:])
				}
				value = string(utf16.Decode(units))
			} else {
				value = string(characters)
			}
			push("\"" + strings.ReplaceAll(value, "\"", "\"\"") + "\"")
		case 0x19: // PtgAttr, optimizer metadata and SUM shortcut.
			payload, err := take(3)
			if err != nil {
				return fail(err)
			}
			flags := payload[0]
			if flags&0x10 != 0 {
				argument, err := pop()
				if err != nil {
					return fail(err)
				}
				push("SUM(" + argument + ")")
			} else if flags&(0x02|0x04|0x08|0x20) != 0 {
				return fail(fmt.Errorf("conditional or assignment PtgAttr 0x%02X is not supported", flags))
			}
		case 0x1C: // PtgErr constant.
			payload, err := take(1)
			if err != nil {
				return fail(err)
			}
			name, ok := biffErrorLiteral(payload[0])
			if !ok {
				return fail(fmt.Errorf("unknown error code 0x%02X", payload[0]))
			}
			push(name)
		case 0x1D: // PtgBool
			payload, err := take(1)
			if err != nil {
				return fail(err)
			}
			if payload[0] > 1 {
				return fail(fmt.Errorf("invalid boolean value %d", payload[0]))
			}
			if payload[0] == 1 {
				push("TRUE()")
			} else {
				push("FALSE()")
			}
		case 0x1E: // PtgInt
			payload, err := take(2)
			if err != nil {
				return fail(err)
			}
			push(strconv.FormatUint(uint64(binary.LittleEndian.Uint16(payload)), 10))
		case 0x1F: // PtgNum
			payload, err := take(8)
			if err != nil {
				return fail(err)
			}
			value := math.Float64frombits(binary.LittleEndian.Uint64(payload))
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return fail(fmt.Errorf("non-finite numeric constant"))
			}
			push(strconv.FormatFloat(value, 'g', -1, 64))
		case 0x20: // PtgArray carries rgcb data outside rgce, unavailable here.
			return fail(fmt.Errorf("array constants require trailing rgcb data"))
		case 0x21: // PtgFunc
			payload, err := take(2)
			if err != nil {
				return fail(err)
			}
			functionID := binary.LittleEndian.Uint16(payload)
			if functionID&0x8000 != 0 {
				return fail(fmt.Errorf("external function table entry %d is not supported", functionID&0x7FFF))
			}
			name, arity, ok := fixedBIFFFunction(functionID)
			if !ok {
				return fail(fmt.Errorf("unknown fixed-arity function id %d", functionID))
			}
			arguments, err := popArguments(&stack, arity)
			if err != nil {
				return fail(err)
			}
			push(name + "(" + strings.Join(arguments, ",") + ")")
		case 0x22: // PtgFuncVar
			payload, err := take(3)
			if err != nil {
				return fail(err)
			}
			count := int(payload[0])
			functionID := binary.LittleEndian.Uint16(payload[1:])
			if functionID&0x8000 != 0 {
				return fail(fmt.Errorf("external function table entry %d is not supported", functionID&0x7FFF))
			}
			name, ok := variableBIFFFunction(functionID)
			if !ok {
				return fail(fmt.Errorf("unknown variable-arity function id %d", functionID))
			}
			arguments, err := popArguments(&stack, count)
			if err != nil {
				return fail(err)
			}
			push(name + "(" + strings.Join(arguments, ",") + ")")
		case 0x23: // PtgName
			payload, err := take(4)
			if err != nil {
				return fail(err)
			}
			index := binary.LittleEndian.Uint32(payload)
			if index == 0 || index > math.MaxUint16 {
				return fail(fmt.Errorf("defined name index %d is out of range", index))
			}
			if d.resolver == nil {
				return fail(fmt.Errorf("defined name %d cannot be resolved", index))
			}
			name, err := d.resolver.DefinedName(uint16(index))
			if err != nil {
				return fail(fmt.Errorf("defined name %d: %w", index, err))
			}
			push(name)
		case 0x24, 0x2C: // PtgRef / PtgRefN
			payload, err := take(4)
			if err != nil {
				return fail(err)
			}
			value, err := d.decodeCellReference(payload, token == 0x2C)
			if err != nil {
				return fail(err)
			}
			push(value)
		case 0x25, 0x2D: // PtgArea / PtgAreaN
			payload, err := take(8)
			if err != nil {
				return fail(err)
			}
			value, err := d.decodeAreaReference(payload, token == 0x2D)
			if err != nil {
				return fail(err)
			}
			push(value)
		case 0x26, 0x27, 0x28, 0x2E: // PtgMem* annotations; references follow separately.
			if _, err := take(6); err != nil {
				return fail(err)
			}
		case 0x29: // PtgMemFunc
			if _, err := take(2); err != nil {
				return fail(err)
			}
		case 0x2A: // PtgRefErr
			if _, err := take(4); err != nil {
				return fail(err)
			}
			push("#REF!")
		case 0x2B: // PtgAreaErr
			if _, err := take(8); err != nil {
				return fail(err)
			}
			push("#REF!")
		case 0x38: // PtgFuncCE
			return fail(fmt.Errorf("macro/add-in function token is not supported"))
		case 0x39: // PtgNameX
			return fail(fmt.Errorf("external defined-name token is not supported"))
		case 0x3A: // PtgRef3d
			payload, err := take(6)
			if err != nil {
				return fail(err)
			}
			value, err := d.decode3DCellReference(payload)
			if err != nil {
				return fail(err)
			}
			push(value)
		case 0x3B: // PtgArea3d
			payload, err := take(10)
			if err != nil {
				return fail(err)
			}
			value, err := d.decode3DAreaReference(payload)
			if err != nil {
				return fail(err)
			}
			push(value)
		case 0x3C: // PtgRefErr3d
			if _, err := take(6); err != nil {
				return fail(err)
			}
			push("#REF!")
		case 0x3D: // PtgAreaErr3d
			if _, err := take(10); err != nil {
				return fail(err)
			}
			push("#REF!")
		default:
			return fail(fmt.Errorf("unsupported formula token"))
		}
	}
	return stack, nil
}

func normalizeFormulaToken(token byte) byte {
	if token >= 0x60 && token <= 0x7F {
		return token - 0x40
	}
	if token >= 0x40 && token <= 0x5F {
		return token - 0x20
	}
	return token
}

func (d *formulaDecoder) decodeCellReference(payload []byte, relativeName bool) (string, error) {
	row := int(binary.LittleEndian.Uint16(payload))
	columnFlags := binary.LittleEndian.Uint16(payload[2:])
	column := int(columnFlags & 0x3FFF)
	rowRelative := columnFlags&0x8000 != 0
	columnRelative := columnFlags&0x4000 != 0
	if relativeName {
		if rowRelative {
			row = d.baseRow + int(int16(uint16(row)))
		}
		if columnRelative {
			column = d.baseColumn + signExtend(columnFlags&0x3FFF, 14)
		}
	} else if d.sharedBy != nil {
		if rowRelative {
			row += d.sharedBy.rows
		}
		if columnRelative {
			column += d.sharedBy.columns
		}
	}
	return formatBIFFReference(row, column, rowRelative, columnRelative)
}

func (d *formulaDecoder) decodeAreaReference(payload []byte, relativeName bool) (string, error) {
	if len(payload) != 8 {
		return "", fmt.Errorf("area reference payload has %d bytes, want 8", len(payload))
	}
	firstRef := []byte{payload[0], payload[1], payload[4], payload[5]}
	lastRef := []byte{payload[2], payload[3], payload[6], payload[7]}
	first, err := d.decodeCellReference(firstRef, relativeName)
	if err != nil {
		return "", err
	}
	last, err := d.decodeCellReference(lastRef, relativeName)
	if err != nil {
		return "", err
	}
	return first + ":" + last, nil
}

func (d *formulaDecoder) decode3DCellReference(payload []byte) (string, error) {
	if d.resolver == nil {
		return "", fmt.Errorf("3D reference cannot be resolved without a workbook resolver")
	}
	index := binary.LittleEndian.Uint16(payload)
	sheet, err := d.resolver.SheetName(index)
	if err != nil {
		return "", fmt.Errorf("external sheet index %d: %w", index, err)
	}
	ref, err := d.decodeCellReference(payload[2:], false)
	if err != nil {
		return "", err
	}
	return quoteBIFFSheet(sheet) + "!" + ref, nil
}

func (d *formulaDecoder) decode3DAreaReference(payload []byte) (string, error) {
	if d.resolver == nil {
		return "", fmt.Errorf("3D area reference cannot be resolved without a workbook resolver")
	}
	index := binary.LittleEndian.Uint16(payload)
	sheet, err := d.resolver.SheetName(index)
	if err != nil {
		return "", fmt.Errorf("external sheet index %d: %w", index, err)
	}
	ref, err := d.decodeAreaReference(payload[2:], false)
	if err != nil {
		return "", err
	}
	return quoteBIFFSheet(sheet) + "!" + ref, nil
}

func formatBIFFReference(row, column int, rowRelative, columnRelative bool) (string, error) {
	if row < 0 || row >= 65536 || column < 0 || column >= 256 {
		return "", fmt.Errorf("cell reference row %d column %d is outside BIFF8 bounds", row+1, column+1)
	}
	columnName, err := excelize.ColumnNumberToName(column + 1)
	if err != nil {
		return "", err
	}
	rowName := strconv.Itoa(row + 1)
	if !columnRelative {
		columnName = "$" + columnName
	}
	if !rowRelative {
		rowName = "$" + rowName
	}
	return columnName + rowName, nil
}

func quoteBIFFSheet(sheet string) string {
	return "'" + strings.ReplaceAll(sheet, "'", "''") + "'"
}

func signExtend(value uint16, bits uint) int {
	shift := 16 - bits
	return int(int16(value<<shift) >> shift)
}

func popArguments(stack *[]string, count int) ([]string, error) {
	if count < 0 || count > len(*stack) {
		return nil, fmt.Errorf("function needs %d arguments but token stack has %d", count, len(*stack))
	}
	start := len(*stack) - count
	arguments := append([]string(nil), (*stack)[start:]...)
	*stack = (*stack)[:start]
	return arguments, nil
}

func fixedBIFFFunction(id uint16) (string, int, bool) {
	function, ok := fixedBIFFFunctions[id]
	return function.name, function.arity, ok
}

func variableBIFFFunction(id uint16) (string, bool) {
	name, ok := variableBIFFFunctions[id]
	return name, ok
}

type fixedBIFFFunctionInfo struct {
	name  string
	arity int
}

var fixedBIFFFunctions = map[uint16]fixedBIFFFunctionInfo{
	2: {"ISNA", 1}, 3: {"ISERROR", 1}, 10: {"NA", 0},
	15: {"SIN", 1}, 16: {"COS", 1}, 17: {"TAN", 1}, 18: {"ATAN", 1},
	19: {"PI", 0}, 20: {"SQRT", 1}, 21: {"EXP", 1}, 22: {"LN", 1},
	23: {"LOG10", 1}, 24: {"ABS", 1}, 25: {"INT", 1}, 26: {"SIGN", 1},
	27: {"ROUND", 2}, 31: {"MID", 3}, 32: {"LEN", 1}, 33: {"VALUE", 1},
	34: {"TRUE", 0}, 35: {"FALSE", 0}, 38: {"NOT", 1}, 39: {"MOD", 2},
	63: {"RAND", 0}, 74: {"NOW", 0}, 98: {"ASIN", 1}, 99: {"ACOS", 1},
	112: {"ISBLANK", 1}, 113: {"ISTEXT", 1}, 114: {"ISNONTEXT", 1},
	115: {"ISLOGICAL", 1}, 118: {"ISREF", 1},
}

var variableBIFFFunctions = map[uint16]string{
	0: "COUNT", 1: "IF", 4: "SUM", 5: "AVERAGE", 6: "MIN", 7: "MAX",
	11: "NPV", 12: "STDEV", 28: "LOOKUP", 29: "INDEX", 30: "REPT",
	36: "AND", 37: "OR", 40: "COUNTA", 48: "TEXT", 64: "MATCH",
	65: "DATE", 66: "TIME", 67: "DAY", 68: "MONTH", 69: "YEAR",
	70: "WEEKDAY", 71: "HOUR", 72: "MINUTE", 73: "SECOND",
	76: "ROWS", 77: "COLUMNS", 78: "OFFSET", 82: "SEARCH",
	101: "HLOOKUP", 102: "VLOOKUP", 109: "LOG", 148: "INDIRECT",
	183: "PRODUCT", 184: "FACT", 197: "TRUNC", 201: "ROUNDUP",
	202: "ROUNDDOWN", 203: "RANK", 204: "ADDRESS", 205: "DAYS360",
	207: "VDB", 208: "MEDIAN", 209: "SUMPRODUCT", 210: "SINH",
	211: "COSH", 212: "TANH", 213: "ASINH", 214: "ACOSH",
	215: "ATANH", 216: "DGET", 220: "DB", 221: "FREQUENCY",
	222: "ERROR.TYPE", 292: "MDETERM", 293: "MINVERSE", 294: "MMULT",
	296: "IPMT", 297: "PPMT", 322: "STDEVP", 323: "VARP",
	339: "ROUNDUP", 340: "ROUNDDOWN", 343: "RANK", 344: "ADDRESS",
	345: "DAYS360", 347: "VDB", 352: "MEDIAN", 353: "SUMPRODUCT",
	354: "SINH", 355: "COSH", 356: "TANH", 357: "ASINH",
	358: "ACOSH", 359: "ATANH", 360: "DGET",
}

func biffErrorLiteral(code byte) (string, bool) {
	literal, ok := map[byte]string{
		0x00: "#NULL!", 0x07: "#DIV/0!", 0x0F: "#VALUE!", 0x17: "#REF!",
		0x1D: "#NAME?", 0x24: "#NUM!", 0x2A: "#N/A",
	}[code]
	return literal, ok
}
