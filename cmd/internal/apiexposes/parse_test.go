package apiexposes

import (
	"reflect"
	"strings"
	"testing"
)

// basicSource is one entity written with every shape the reader understands:
// a fragment assembled over lines, an alias, two names in one expose, a
// condition, a with_options scope, a nesting block holding a value block, a
// brace value block, a lambda whose body is a do block, an expose continued
// over three lines that ends in a value block, references in the three
// spellings, a merge, a statement-level if and unless around exposes, a
// string name, a backslash continuation, and the bodies the reader skips: a
// method with an if expression inside, an endless method, a singleton class.
const basicSource = `# frozen_string_literal: true

module API
  module Entities
    class Basic < Grape::Entity
      include ::API::Helpers::RelatedResourcesHelpers

      expose :id, documentation: { type: 'Integer', example: 1 }
      expose :name, as: :title, documentation: { type: 'String' } # renamed
      expose :a, :b
      expose :hidden, if: ->(_, options) { options[:hidden] }
      with_options if: ->(_, _) { user_has_access? } do
        expose :readme_url
        expose :forks_count, if: ->(p, _) { p.forks? }
      end
      expose :_links, documentation: { type: 'Hash' } do
        expose :self do |obj|
          expose_url(obj)
        end
        expose :issues, if: ->(p, o) { issues_available?(p, o) } do |obj|
          if obj.issues?
            expose_url(obj)
          end
        end
      end
      expose(:calculated, documentation: { type: 'Integer' }) { |obj, _| obj.count }
      expose :task_status, if: ->(issue, _) do
        !issue.tasks?
      end
      expose :projects,
        if: ->(_, options) { options[:with_projects] },
        using: ::API::Entities::Child do |group, options|
        group.projects.each do |project|
          project
        end
      end
      expose :child, using: Entities::Child
      expose :bits, merge: true, using: Child
      expose :job, with: Ci::Job
      expose :raw_name
      expose 'quoted'
      expose :described,
        documentation: { desc: 'One of "A", "B", ' \
          'and "C"' }
      if Gitlab.ee?
        expose :only_ee_build
      end
      unless Rails.env.test?
        expose :not_in_tests
        expose :not_in_tests_or_ci, unless: ->(_, _) { ci? }
      end
      with_options(format_with: :time_tracking_formatter) do
        expose :total_time, as: :human_total_time
      end
      expose :missing_ref, using: Entities::Missing
      expose :shard, using: StorageShardEntity
      expose(:trailing, documentation: { type: 'String' },)
      expose :escaped, documentation: { example: 'it\'s' }
      expose(:nested_parens, documentation: { example: f(1) })
      expose :deep, if: ->(obj, _) do
        if obj.a?
          true
        else
          false
        end
      end
      expose(*Helpers.mirror_attributes, if: ->(_, _) { mirror? })

      private

      def helper
        result = if object.thing?
                   1
                 else
                   2
                 end
        result
      end

      def endless = 1

      class << self
        def preload(relation)
          relation
        end
      end
    end
  end
end
`

// childSource declares the entity Basic refers to, a nested module, an
// option-symbol condition, and a reference nothing declares.
const childSource = `module API
  module Entities
    class Child < Basic
      expose :extra, if: :with_extra
      expose :serialized, with: ::DetailedStatusEntity
    end

    module Ci
      class Job < Grape::Entity
        expose :status
      end
    end
  end
end
`

// epicSource is an entity only Enterprise declares, with a licensed
// condition.
const epicSource = `module API
  module Entities
    class Epic < Grape::Entity
      expose :id
      expose :gated, if: ->(epic, _) { epic.feature_available?(:epics) }
    end
  end
end
`

// prependSource is the Enterprise module prepended into Basic.
const prependSource = `module EE
  module API
    module Entities
      module Basic
        extend ActiveSupport::Concern

        class_methods do
          def preload(relation)
            super.with_more
          end
        end

        prepended do
          expose :licensed, if: ->(p, _) { p.licensed_feature_available?(:merge_pipelines) }
          expose :ultimate_thing, if: ->(_, _) { ::License.feature_available?(:security_orchestration_policies) }
          expose :setting, if: ->(p, _) { p.feature_available?(:issues) }
        end
      end
    end
  end
end
`

// orphanSource prepends into an entity no Community file declares.
const orphanSource = `module EE
  module API
    module Entities
      module Orphan
        prepended do
          expose :only_here
        end
      end
    end
  end
end
`

// helperSource is a class outside API::Entities, which is not an entity.
const helperSource = `module API
  module Helpers
    class Presenter
      expose :nothing
      def present
        1
      end
    end
  end
end
`

// straySource holds the places an expose or a prepend can sit without an
// entity to belong to: a class outside any module, a prepend under a module
// that is not Enterprise's, and an expose directly in a module.
const straySource = `class Standalone
end

module API
  prepended do
    expose :nope
  end

  module Entities
    expose :stray

    class Stray < Grape::Entity
      expose :id
    end
  end
end
`

// testFeatures is a feature table naming the symbols the fixtures ask about.
const testFeatures = `module GitlabSubscriptions
  class Features
    GLOBAL_FEATURES = %i[
      elastic_search
    ].freeze

    STARTER_FEATURES = %i[
      merge_pipelines
    ].freeze

    PREMIUM_FEATURES = %i[
      epics
      merge_pipelines
    ].freeze

    ULTIMATE_FEATURES = %i[
      security_orchestration_policies
    ].freeze

    ALL_PREMIUM_FEATURES = STARTER_FEATURES + PREMIUM_FEATURES
  end
end
`

// fixtureFiles is the whole fixture tree, keyed the way the generator keys
// what it fetched.
func fixtureFiles() map[string][]byte {
	return map[string][]byte{
		"lib/api/entities/basic.rb":         []byte(basicSource),
		"lib/api/entities/child.rb":         []byte(childSource),
		"ee/lib/api/entities/epic.rb":       []byte(epicSource),
		"ee/lib/ee/api/entities/basic.rb":   []byte(prependSource),
		"ee/lib/ee/api/entities/orphan.rb":  []byte(orphanSource),
		"lib/api/helpers/presenter.rb":      []byte(helperSource),
		"lib/api/entities/stray.rb":         []byte(straySource),
		"lib/api/entities/empty_comment.rb": []byte("# nothing here\n"),
	}
}

// parseFixture parses the fixture tree against the fixture feature table.
func parseFixture(t *testing.T) (map[string]Entity, int) {
	t.Helper()
	entities, exposes, err := Parse(fixtureFiles(), ParseFeatures([]byte(testFeatures)))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return entities, exposes
}

// names lists the field names of a list, nested ones excluded.
func names(fields []Field) []string {
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		out = append(out, field.Name)
	}
	return out
}

// TestParse_EveryShapeGitLabWrites_IsReadAsGrapeWouldReadIt verifies the
// reader against one entity written with every shape the repository's
// entities use, checking what each shape must yield: the name after `as:`,
// the condition as written, a scope's condition joined onto its fields, a
// nesting block's fields under their parent, a value block skipped whole, a
// do-bodied lambda read to its end, references resolved in every spelling,
// and the bodies that hold no fields passed over without losing count.
func TestParse_EveryShapeGitLabWrites_IsReadAsGrapeWouldReadIt(t *testing.T) {
	entities, _ := parseFixture(t)

	basic, ok := entities["APIEntitiesBasic"]
	if !ok {
		t.Fatalf("Parse() entities = %v, want APIEntitiesBasic among them", keysOf(entities))
	}
	if basic.File != "lib/api/entities/basic.rb" || basic.Line != 5 || basic.Parent != "" || basic.Edition != "" {
		t.Errorf("Basic = %+v, want declared at basic.rb:5 with no parent and no edition", basic)
	}
	want := []string{
		"id", "title", "a", "b", "hidden", "readme_url", "forks_count", "_links", "calculated", "task_status",
		"projects", "child", "bits", "job", "raw_name", "quoted", "described", "only_ee_build", "not_in_tests",
		"not_in_tests_or_ci", "human_total_time", "missing_ref", "shard", "trailing", "escaped", "nested_parens", "deep",
		"*Helpers.mirror_attributes", "licensed", "ultimate_thing", "setting",
	}
	if got := names(basic.Fields); !reflect.DeepEqual(got, want) {
		t.Errorf("Basic fields = %v, want %v", got, want)
	}
	byName := map[string]Field{}
	for _, field := range basic.Fields {
		byName[field.Name] = field
	}
	cases := []struct {
		name string
		want Field
	}{
		{name: "an alias renames the field", want: Field{Name: "title", Line: 9}},
		{name: "a condition is kept as written", want: Field{Name: "hidden", Line: 11, If: "->(_, options) { options[:hidden] }"}},
		{name: "a scope's condition reaches its fields", want: Field{Name: "readme_url", Line: 13, If: "->(_, _) { user_has_access? }"}},
		{name: "a scope's condition joins the field's own", want: Field{Name: "forks_count", Line: 14, If: "(->(_, _) { user_has_access? }) && (->(p, _) { p.forks? })"}},
		{name: "a brace value block declares one field", want: Field{Name: "calculated", Line: 26}},
		{name: "a do-bodied lambda is read to its end", want: Field{Name: "task_status", Line: 27, If: "->(issue, _) do !issue.tasks? end"}},
		{name: "a three-line expose ending in a value block", want: Field{Name: "projects", Line: 30, If: "->(_, options) { options[:with_projects] }", Using: "APIEntitiesChild"}},
		{name: "Entities::X resolves under API", want: Field{Name: "child", Line: 37, Using: "APIEntitiesChild"}},
		{name: "a bare name resolves in the namespace, with merge", want: Field{Name: "bits", Line: 38, Using: "APIEntitiesChild", Merge: true}},
		{name: "with: resolves through a nested module", want: Field{Name: "job", Line: 39, Using: "APIEntitiesCiJob"}},
		{name: "a string name", want: Field{Name: "quoted", Line: 41}},
		{name: "a backslash continuation is one statement", want: Field{Name: "described", Line: 42}},
		{name: "a statement-level if is a scope", want: Field{Name: "only_ee_build", Line: 46, If: "Gitlab.ee?"}},
		{name: "a statement-level unless is a scope", want: Field{Name: "not_in_tests", Line: 49, Unless: "Rails.env.test?"}},
		{name: "an unless scope joins the field's own unless with or", want: Field{Name: "not_in_tests_or_ci", Line: 50, Unless: "(Rails.env.test?) || (->(_, _) { ci? })"}},
		{name: "a with_options without a condition scopes nothing", want: Field{Name: "human_total_time", Line: 53}},
		{name: "Entities::X nothing declares is read as under API", want: Field{Name: "missing_ref", Line: 55, Using: "APIEntitiesMissing"}},
		{name: "a reference nothing declares is kept as written", want: Field{Name: "shard", Line: 56, Using: "StorageShardEntity"}},
		{name: "a trailing comma in parentheses", want: Field{Name: "trailing", Line: 57}},
		{name: "an escaped quote inside a string", want: Field{Name: "escaped", Line: 58}},
		{name: "parentheses nested in the arguments", want: Field{Name: "nested_parens", Line: 59}},
		{name: "a do-bodied lambda holding an if of its own", want: Field{Name: "deep", Line: 60, If: "->(obj, _) do if obj.a?; true; else; false; end end"}},
		{name: "a splat names its fields at run time", want: Field{Name: "*Helpers.mirror_attributes", Line: 67, If: "->(_, _) { mirror? }", Splat: true}},
		{name: "a prepended field carries its file, edition, feature and tier", want: Field{Name: "licensed", File: "ee/lib/ee/api/entities/basic.rb", Line: 14, Edition: "ee", If: "->(p, _) { p.licensed_feature_available?(:merge_pipelines) }", Features: []string{"merge_pipelines"}, Tier: TierPremium}},
		{name: "License.feature_available? resolves too", want: Field{Name: "ultimate_thing", File: "ee/lib/ee/api/entities/basic.rb", Line: 15, Edition: "ee", If: "->(_, _) { ::License.feature_available?(:security_orchestration_policies) }", Features: []string{"security_orchestration_policies"}, Tier: TierUltimate}},
		{name: "a project feature is named and has no tier", want: Field{Name: "setting", File: "ee/lib/ee/api/entities/basic.rb", Line: 16, Edition: "ee", If: "->(p, _) { p.feature_available?(:issues) }", Features: []string{"issues"}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, found := byName[testCase.want.Name]
			if !found {
				t.Fatalf("field %q missing", testCase.want.Name)
			}
			got.Nested = nil
			if !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("field = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

// TestParse_EntitiesAndTheirRelations_AreRecorded verifies what the fixture
// says about entities rather than fields: a nesting block's fields under
// their parent, a child naming its parent, a reference nothing declares kept
// as written, a class in a nested module, an Enterprise entity, an entity
// made from its prepend alone, a class outside the namespace left out, and
// the count of everything read.
func TestParse_EntitiesAndTheirRelations_AreRecorded(t *testing.T) {
	entities, exposes := parseFixture(t)
	byName := map[string]Field{}
	for _, field := range entities["APIEntitiesBasic"].Fields {
		byName[field.Name] = field
	}

	links := byName["_links"]
	if got := names(links.Nested); !reflect.DeepEqual(got, []string{"self", "issues"}) {
		t.Errorf("_links nested = %v, want [self issues]", got)
	}
	if links.Nested[1].If != "->(p, o) { issues_available?(p, o) }" {
		t.Errorf("_links.issues condition = %q", links.Nested[1].If)
	}

	child := entities["APIEntitiesChild"]
	if child.Parent != "APIEntitiesBasic" {
		t.Errorf("Child parent = %q, want APIEntitiesBasic", child.Parent)
	}
	if got := names(child.Fields); !reflect.DeepEqual(got, []string{"extra", "serialized"}) {
		t.Errorf("Child fields = %v", got)
	}
	if child.Fields[0].If != ":with_extra" || child.Fields[1].Using != "DetailedStatusEntity" {
		t.Errorf("Child fields = %+v, want the symbol condition and the unresolved reference kept as written", child.Fields)
	}
	if job := entities["APIEntitiesCiJob"]; job.Line != 9 || names(job.Fields)[0] != "status" {
		t.Errorf("Ci::Job = %+v, want the nested module's class", job)
	}
	epic := entities["APIEntitiesEpic"]
	if epic.Edition != "ee" || epic.Fields[1].Tier != TierPremium {
		t.Errorf("Epic = %+v, want an Enterprise entity whose gated field is premium", epic)
	}
	orphan := entities["APIEntitiesOrphan"]
	if orphan.Edition != "ee" || orphan.File != "ee/lib/ee/api/entities/orphan.rb" || names(orphan.Fields)[0] != "only_here" {
		t.Errorf("Orphan = %+v, want an entity made from its prepend alone", orphan)
	}
	if _, found := entities["APIHelpersPresenter"]; found {
		t.Error("a class outside API::Entities was recorded as an entity")
	}
	if _, found := entities["Standalone"]; found {
		t.Error("a class outside any module was recorded as an entity")
	}
	if got := names(entities["APIEntitiesStray"].Fields); !reflect.DeepEqual(got, []string{"id"}) {
		t.Errorf("Stray fields = %v, want [id]: the expose in its module and the prepend under API belong to nothing", got)
	}
	if exposes != 40 {
		t.Errorf("Parse() exposes = %d, want 40 (nested ones counted, the splat as one)", exposes)
	}
}

// keysOf lists a map's keys for a message.
func keysOf(entities map[string]Entity) []string {
	out := make([]string, 0, len(entities))
	for name := range entities {
		out = append(out, name)
	}
	return out
}

// TestParse_SourceThisReaderCannotFollow_IsRefused verifies that a shape the
// reader does not understand stops the run with the file and line, rather than
// leaving every frame after it off by one and a record that is quietly wrong.
func TestParse_SourceThisReaderCannotFollow_IsRefused(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{name: "an end with nothing open", source: "module API\n  module Entities\n  end\nend\nend\n", want: "x.rb:5: end with nothing open"},
		{name: "a block left open", source: "module API\n  module Entities\n    class Basic < Grape::Entity\n      expose :id\n  end\n", want: "x.rb: 2 block(s) left open at the end of the file, the last a module Entities"},
		{name: "a class declaration it cannot read", source: "module API\n  module Entities\n    class Basic <\n    end\n  end\nend\n", want: "x.rb:3: class declaration this reader does not understand"},
		{name: "a module declaration it cannot read", source: "module api\nend\n", want: "x.rb:1: module declaration this reader does not understand"},
		{name: "an expose without a name", source: "module API\n  module Entities\n    class Basic < Grape::Entity\n      expose documentation: { type: 'String' }\n    end\n  end\nend\n", want: "x.rb:4: expose without a field name"},
		{name: "an expose with an unclosed parenthesis", source: "module API\n  module Entities\n    class Basic < Grape::Entity\n      expose(:id\n    end\n  end\nend\n", want: "x.rb:4: expose with an unclosed parenthesis"},
		{name: "a lambda body the file ends inside", source: "module API\n  module Entities\n    class Basic < Grape::Entity\n      expose :x, if: ->(a) do\n        a.b?\n", want: "x.rb: 3 block(s) left open at the end of the file, the last a class Basic"},
		{name: "no entity at all", source: "module API\n  module Helpers\n  end\nend\n", want: "no entity declared under API::Entities in 1 file(s)"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, _, err := Parse(map[string][]byte{"x.rb": []byte(testCase.source)}, nil)

			if err == nil {
				t.Fatalf("Parse() error = nil, want one saying %q", testCase.want)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Parse() error = %q, want it to say %q", err, testCase.want)
			}
		})
	}
}

// TestParse_FrameDescriptions_NameWhatWasLeftOpen verifies the words an
// unclosed frame is reported with, one per kind, since the message is what a
// reader uses to find the construct the reader stumbled on.
func TestParse_FrameDescriptions_NameWhatWasLeftOpen(t *testing.T) {
	cases := []struct {
		name string
		f    frame
		want string
	}{
		{name: "module", f: frame{kind: frameModule, name: "API"}, want: "module API"},
		{name: "class", f: frame{kind: frameClass, name: "Basic"}, want: "class Basic"},
		{name: "prepended", f: frame{kind: framePrepended}, want: "prepended block"},
		{name: "nesting", f: frame{kind: frameNesting}, want: "nesting expose block"},
		{name: "scope", f: frame{kind: frameScope}, want: "with_options or if scope"},
		{name: "skip", f: frame{kind: frameSkip}, want: "method or value block"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.f.describe(); got != testCase.want {
				t.Errorf("describe() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestParse_ExposesOutsideAnyEntity_AreNotRecorded verifies the two places an
// expose can sit without an entity to belong to: a prepend whose module is not
// under EE::API::Entities, and a nesting block reached inside a skipped body.
// Both are read past rather than attached to something, and the file still has
// to hold an entity for the run to succeed.
func TestParse_ExposesOutsideAnyEntity_AreNotRecorded(t *testing.T) {
	source := "module EE\n  module Helpers\n    module Thing\n      prepended do\n        expose :stray\n      end\n    end\n  end\nend\n" +
		"module API\n  module Entities\n    class Basic < Grape::Entity\n      expose :id\n      def helper\n        expose :inside_method do\n          expose :deeper\n        end\n      end\n    end\n  end\nend\n"

	entities, exposes, err := Parse(map[string][]byte{"lib/api/entities/basic.rb": []byte(source)}, nil)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := names(entities["APIEntitiesBasic"].Fields); !reflect.DeepEqual(got, []string{"id"}) {
		t.Errorf("Basic fields = %v, want [id] alone", got)
	}
	if exposes != 1 {
		t.Errorf("Parse() exposes = %d, want 1", exposes)
	}
}
