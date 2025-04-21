func detectDateComparisonOperator(db *gorm.DB, field string, values []string) *gorm.DB {
	if len(values) == 0 {
		return db
	}

	if len(values) == 2 {
		value := values[0]
		if len(value) >= 5 && value[0:4] == "=>=<" {
			gtValue := value[4:]
			gtDate, _ := time.Parse(time.RFC3339, gtValue)
			ltValue := values[1]
			ltDate, _ := time.Parse(time.RFC3339, ltValue)
			return db.Where(fmt.Sprintf("%s >= ? AND %s <= ?", field, field), gtDate, ltDate)
		}
	}

	if len(values) > 1 {
		dates := []time.Time{}
		for _, v := range values {
			dv, _ := time.Parse(time.RFC3339, v)
			dates = append(dates, dv)
		}
		return db.Where(fmt.Sprintf("%s IN ?", field), dates)
	}

	value := values[0]
	var oper string
	original := value

	if len(value) >= 3 {
		var uv string
		if value[0:2] == "<=" {
			oper = "<="
			uv = value[2:]
		}
		if value[0:2] == ">=" {
			oper = ">="
			uv = value[2:]
		}
		if value[0:2] == "!=" {
			oper = "<>"
			uv = value[2:]
		}
		if uv != "" {
			value = uv
		}
	}

	if len(value) >= 2 {
		var uv string
		if value[0:1] == "<" {
			oper = "<"
			uv = value[1:]
		}
		if value[0:1] == ">" {
			oper = ">"
			uv = value[1:]
		}
		if value[0:1] == "-" {
			oper = "<>"
			uv = value[1:]
		}
		if uv != "" {
			value = uv
		}
	}

	if reNull.MatchString(value) {
		if oper != "" {
			return db.Where(fmt.Sprintf("%s %s NULL", field, oper))
		}
		return db.Where(fmt.Sprintf("%s IS NULL", field))
	}

	dv, _ := time.Parse(time.RFC3339, value)

	if oper != "" {
		return db.Where(fmt.Sprintf("%s %s ?", field, oper), dv)
	}

	return db.Where(fmt.Sprintf("%s = ?", field), dv)
}


func detectNumericComparisonOperator(db *gorm.DB, field string, values []string, numericType string) *gorm.DB {
	// fmt.Printf("fffffffffffffffffffff values: %v\n", values)
	if len(values) == 0 {
		return db
	}

	if len(values) == 2 {
		value := values[0]
		if len(value) >= 5 && value[0:4] == "=>=<" {
			gtValue := value[4:]
			ltValue := values[1]

			// range query
			return db.Where(fmt.Sprintf("%s >= ? AND %s <= ?", field, field), gtValue, ltValue)
		}
	}

	// handle when values is an array
	if len(values) > 1 {
		// return IN clause
		return db.Where(fmt.Sprintf("%s IN ?", field), values)
	}

	value := values[0]
	var oper string

	// fmt.Printf("fffffffffffffffffffff value: %v\n", value)
	// check if string value is long enough for a 2 char prefix
	if len(value) >= 3 {
		var uv string

		// lte
		if value[0:2] == "<=" {
			oper = "<="
			uv = value[2:]
		}

		// gte
		if value[0:2] == ">=" {
			oper = ">="
			uv = value[2:]
		}

		// ne
		if value[0:2] == "!=" {
			oper = "<>"
			uv = value[2:]
		}

		// update value to remove the prefix
		if uv != "" {
			value = uv
		}
	}

	// check if string value is long enough for a single char prefix
	if len(value) >= 2 {
		var uv string

		// lt
		if value[0:1] == "<" {
			oper = "<"
			uv = value[1:]
		}

		// gt
		if value[0:1] == ">" {
			oper = ">"
			uv = value[1:]
		}

		// update value to remove the prefix
		if uv != "" {
			value = uv
		}
	}

	if reNull.MatchString(value) {
		// detect $ne operator (note use of - shorthand here which is not
		// processed on numeric values that are not "null")
		if value[0:1] == "-" || value[0:2] == "!=" {
			oper = "<>"
		}

		if oper != "" {
			// return with the specified operator
			return db.Where(fmt.Sprintf("%s %s NULL", field, oper))
		}

		return db.Where(fmt.Sprintf("%s IS NULL", field))
	}

	if oper != "" {
		// return with the specified operator
		return db.Where(fmt.Sprintf("%s %s ?", field, oper), value)
	}

	// no operator... just the value
	return db.Where(fmt.Sprintf("%s = ?", field), value)
}


func detectStringComparisonOperator(field string, values []string, dataType string) map[string]interface{} {
	// fmt.Printf("fffffffffffffffffffff values: %v\n", values)
	if len(values) == 0 {
		return nil
	}

	// Handle `object` type with existence checks
	if dataType == "object" {
		filter := map[string]interface{}{}

		for _, fn := range values {
			exists := true

			// check for "-" prefix
			if len(fn) >= 2 && fn[0:1] == "-" {
				exists = false
				fn = fn[1:]
			}

			// check for "!=" prefix
			if exists && len(fn) >= 3 && fn[0:2] == "!=" {
				exists = false
				fn = fn[2:]
			}

			fullField := fmt.Sprintf("%s.%s", field, fn)
			filter[fullField] = map[string]interface{}{
				"$exists": exists,
			}
		}

		return filter
	}

	// Multiple values: use $in logic
	if len(values) > 1 {
		a := make([]interface{}, 0, len(values))
		for _, v := range values {
			a = append(a, v)
		}

		if dataType == "array" {
			return map[string]interface{}{field: a}
		}

		return map[string]interface{}{field: map[string]interface{}{
			"$in": a,
		}}
	}

	// Single value
	value := values[0]
	if !reWord.MatchString(value) {
		return nil
	}

	bw := false
	c := false
	em := false
	ew := false
	ne := false

	if len(value) > 1 {
		bw = value[len(value)-1:] == "*"
		ew = value[0:1] == "*"
		c = bw && ew
		ne = value[0:1] == "-"

		if ne || ew {
			value = value[1:]
		}

		if bw {
			value = value[:len(value)-1]
		}

		if c {
			bw = false
			ew = false
		}
	}

	if len(value) > 2 && !ne {
		ne = value[0:2] == "!="
		em = value[0:1] == `"` && value[len(value)-1:] == `"`

		if ne {
			value = value[2:]
		}

		if em {
			value = value[1 : len(value)-1]
		}
	}

	// handle null keyword
	if reNull.MatchString(value) {
		if ne {
			return map[string]interface{}{field: map[string]interface{}{
				"$ne": nil,
			}}
		}
		return map[string]interface{}{field: nil}
	}

	if ne {
		return map[string]interface{}{field: map[string]interface{}{
			"$ne": value,
		}}
	}

	if c {
		return map[string]interface{}{field: map[string]interface{}{
			"$regex":   value,
			"$options": "i", // simulate case-insensitive match
		}}
	}

	if bw {
		return map[string]interface{}{field: map[string]interface{}{
			"$regex":   fmt.Sprintf("^%s", value),
			"$options": "i",
		}}
	}

	if ew {
		return map[string]interface{}{field: map[string]interface{}{
			"$regex":   fmt.Sprintf("%s$", value),
			"$options": "i",
		}}
	}

	if em {
		return map[string]interface{}{field: map[string]interface{}{
			"$regex": fmt.Sprintf("^%s$", value),
		}}
	}

	return map[string]interface{}{field: value}
}
