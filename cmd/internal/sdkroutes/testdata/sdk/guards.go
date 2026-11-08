// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gitlab

// The templates the nil checks below choose between.
var (
	routeGuardedItem  = route("guarded/%s/items/%s")
	routeGuardedItems = route("guarded/%s/items")
)

// GuardedAppend is the shape the label update and delete take: the request
// options built up, and the path chosen by a nil check of a parameter.
func (s *GuardsService) GuardedAppend(pid, lid any) (*Issue, *Response, error) {
	reqOpts := make([]doOption, 0, 4)
	if lid != nil {
		reqOpts = append(reqOpts, withPath(routeGuardedItem, ProjectID{pid}, LabelID{lid}))
	} else {
		reqOpts = append(reqOpts, withPath(routeGuardedItems, ProjectID{pid}))
	}
	return do[*Issue](s.client, reqOpts...)
}

// GuardedEquals is the same check written the other way round, with an early
// return and no else.
func (s *GuardsService) GuardedEquals(pid, lid any) (*Issue, *Response, error) {
	if lid == nil {
		return do[*Issue](s.client, withPath(routeGuardedItems, ProjectID{pid}))
	}
	return do[*Issue](s.client, withPath(routeGuardedItem, ProjectID{pid}, lid))
}

// GuardedEqualsElse is that check with an else, whose side is the other one.
func (s *GuardsService) GuardedEqualsElse(pid, lid any) (*Issue, *Response, error) {
	if lid == nil {
		return do[*Issue](s.client, withPath(routeGuardedItems, ProjectID{pid}))
	} else {
		return do[*Issue](s.client, withPath(routeGuardedItem, ProjectID{pid}, lid))
	}
}

// GuardedElseIf chooses in an else that holds another if, which its check's
// else encloses.
func (s *GuardsService) GuardedElseIf(pid, lid any) (*Issue, *Response, error) {
	if lid != nil {
		return do[*Issue](s.client, withPath(routeGuardedItem, ProjectID{pid}, lid))
	} else if pid != nil {
		return do[*Issue](s.client, withPath(routeGuardedItems, ProjectID{pid}))
	}
	return nil, nil, nil
}

// GuardedNested sits behind two checks at once.
func (s *GuardsService) GuardedNested(pid, lid any) (*Issue, *Response, error) {
	if pid != nil {
		if lid != nil {
			return do[*Issue](s.client, withPath(routeGuardedItem, ProjectID{pid}, lid))
		}
	}
	return nil, nil, nil
}

// GuardedAlsoOutside sends one route behind a check and also outside it, so the
// check does not keep the route from being sent.
func (s *GuardsService) GuardedAlsoOutside(pid, lid any) (*Issue, *Response, error) {
	if lid != nil {
		_, _, _ = do[*Issue](s.client, withPath(routeGuardedItems, ProjectID{pid}))
	}
	return do[*Issue](s.client, withPath(routeGuardedItems, ProjectID{pid}))
}

// GuardedOutsideFirst is the same with the way outside any check met first.
func (s *GuardsService) GuardedOutsideFirst(pid, lid any) (*Issue, *Response, error) {
	_, _, _ = do[*Issue](s.client, withPath(routeGuardedItems, ProjectID{pid}))
	if lid != nil {
		return do[*Issue](s.client, withPath(routeGuardedItems, ProjectID{pid}))
	}
	return nil, nil, nil
}

// GuardedTwoWays sends one route behind two checks in turn, which of them
// neither alone keeps from being sent, and behind one check they share.
func (s *GuardsService) GuardedTwoWays(pid, lid, other any) (*Issue, *Response, error) {
	if other != nil {
		if pid != nil {
			return do[*Issue](s.client, withPath(routeGuardedItems, ProjectID{pid}))
		}
		if lid != nil {
			return do[*Issue](s.client, withPath(routeGuardedItems, ProjectID{pid}))
		}
	}
	return nil, nil, nil
}

// GuardedAssigned checks a parameter its body assigns.
func (s *GuardsService) GuardedAssigned(pid, lid any) (*Issue, *Response, error) {
	lid = wrap(lid)
	if lid != nil {
		return do[*Issue](s.client, withPath(routeGuardedItem, ProjectID{pid}, lid))
	}
	return nil, nil, nil
}

// GuardedLocal checks a name that is no parameter.
func (s *GuardsService) GuardedLocal(pid any) (*Issue, *Response, error) {
	s.client.field = opaque()
	local := wrap(pid)
	if local != nil {
		return do[*Issue](s.client, withPath(routeGuardedItem, ProjectID{pid}, local))
	}
	return nil, nil, nil
}

// GuardedGlobal checks a name the package declares, which no body assigns.
func (s *GuardsService) GuardedGlobal(pid any) (*Issue, *Response, error) {
	if notARoute != nil {
		return do[*Issue](s.client, withPath(routeGuardedItems, ProjectID{pid}))
	}
	return nil, nil, nil
}

// GuardedField checks what a selector names, which names no parameter.
func (s *GuardsService) GuardedField(pid any) (*Issue, *Response, error) {
	if s.client != nil {
		return do[*Issue](s.client, withPath(routeGuardedItems, ProjectID{pid}))
	}
	return nil, nil, nil
}

// GuardedTerms checks several things at once.
func (s *GuardsService) GuardedTerms(pid, lid any) (*Issue, *Response, error) {
	if lid != nil && pid != nil {
		return do[*Issue](s.client, withPath(routeGuardedItem, ProjectID{pid}, lid))
	}
	return nil, nil, nil
}

// GuardedNilFirst writes nil first.
func (s *GuardsService) GuardedNilFirst(pid, lid any) (*Issue, *Response, error) {
	if nil != lid {
		return do[*Issue](s.client, withPath(routeGuardedItem, ProjectID{pid}, lid))
	}
	return nil, nil, nil
}

// GuardedOtherName compares two parameters.
func (s *GuardsService) GuardedOtherName(pid, lid any) (*Issue, *Response, error) {
	if lid == pid {
		return do[*Issue](s.client, withPath(routeGuardedItem, ProjectID{pid}, lid))
	}
	return nil, nil, nil
}

// GuardedOtherValue compares a parameter with a value.
func (s *GuardsService) GuardedOtherValue(pid any, lid int64) (*Issue, *Response, error) {
	if lid != 0 {
		return do[*Issue](s.client, withPath(routeGuardedItem, ProjectID{pid}, lid))
	}
	return nil, nil, nil
}

// GuardedOtherOperator compares a parameter some other way.
func (s *GuardsService) GuardedOtherOperator(pid any, lid *int64) (*Issue, *Response, error) {
	if lid >= nil {
		return do[*Issue](s.client, withPath(routeGuardedItem, ProjectID{pid}, lid))
	}
	return nil, nil, nil
}

// GuardedCondition checks no comparison at all.
func (s *GuardsService) GuardedCondition(pid any, lid bool) (*Issue, *Response, error) {
	if lid {
		return do[*Issue](s.client, withPath(routeGuardedItems, ProjectID{pid}))
	}
	return nil, nil, nil
}

// GuardedDelegate hands the path to a helper behind a check, and the helper
// checks a parameter of its own that no caller of the method hands.
func (s *GuardsService) GuardedDelegate(pid, lid any) (*Issue, *Response, error) {
	if lid != nil {
		return s.guardedHelper(pid, lid)
	}
	return nil, nil, nil
}

func (s *GuardsService) guardedHelper(pid, id any) (*Issue, *Response, error) {
	if id != nil {
		return do[*Issue](s.client, withPath(routeGuardedItem, ProjectID{pid}, id))
	}
	return nil, nil, nil
}

// GuardedLegacy builds its request the pre-option way behind a check, whose
// path the reading folds without asking which branch it is written in.
func (s *GuardsService) GuardedLegacy(lid any) (*Issue, *Response, error) {
	if lid != nil {
		_, err := s.client.NewRequest(http.MethodGet, "guarded/legacy", nil, nil)
		return nil, nil, err
	}
	return nil, nil, nil
}
