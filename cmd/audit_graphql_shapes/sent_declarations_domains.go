package main

// The answers for five GraphQL domains: branch rules, the CI/CD catalog,
// custom emoji, and security attributes and categories.
//
// Each field the pinned schema offered at an object these packages decode, and
// no document of theirs selected, was triaged. The ones a caller needs are
// selected and published: who may push, merge and unprotect a branch, the
// security-policy flags of a protection, an approval rule's eligible approvers,
// a catalog version's README, author and commit, an input's options, pattern
// and conditional rules, and the attributes a category takes with it when it
// is deleted. What is left is answered below, each with the reason a reviewer
// can check against GitLab's own GraphQL reference or resolvers.

// Where the findings answered here are filed.
const (
	branchRulesPackage        = toolsDir + "/branchrules"
	ciCatalogPackage          = toolsDir + "/cicatalog"
	customEmojiPackage        = toolsDir + "/customemoji"
	securityAttributesPackage = toolsDir + "/securityattributes"
	securityCategoriesPackage = toolsDir + "/securitycategories"
)

// userReferenceReason is why a user named by something other than a note is
// named with its identity and nothing more. role says what the user is to the
// response, and selection what the document selects of it.
func userReferenceReason(role, selection string) string {
	return "The object is " + role + ", named so a reader knows who it is, and the document selects " +
		selection + " of it. Its own fields are the users domain's surface, which publishes them through " +
		"gitlab_user, and answering them here would be that domain a second time through a document nobody " +
		"maintains against it."
}

// branchRuleExperimentReason is why a branch rule field GitLab still marks as
// an experiment is left out of both branch rule documents.
func branchRuleExperimentReason(field, introduced, meaning string) string {
	return "GitLab's GraphQL API reference marks " + field + " (" + meaning + ") \"Introduced in GitLab " +
		introduced + ". Status: Experiment.\" GitLab may change or remove an experiment without notice and " +
		"refuses a whole document naming a field it no longer has, so selecting it would stake every branch " +
		"rule listing on the experiment. It is selected once GitLab declares it generally available."
}

// domainSentDeclarations answers the sent findings of the five domains.
func domainSentDeclarations() []sentDeclaration {
	declarations := append(branchRuleDeclarations(), ciCatalogDeclarations()...)
	return append(declarations, securityAndEmojiDeclarations()...)
}

// securityAndEmojiDeclarations answers what the security attribute and
// category mutations and the custom emoji listing leave out: the ids a payload
// hands back that the caller sent, the objects reached again by walking back
// to their parent, and the viewer's permissions on an emoji.
func securityAndEmojiDeclarations() []sentDeclaration {
	return []sentDeclaration{
		{
			Package:    securityAttributesPackage,
			SchemaType: "SecurityAttributeDestroyPayload",
			Field:      "deletedAttributeGid",
			Category:   categoryRestated,
			Reason: "The global id of the deleted attribute, which is the id the caller sent: " +
				"Mutations::Security::Attributes::Destroy finds the attribute by that argument and hands its id back " +
				"on success, and nil only beside the errors the decoder reads. The confirmation already names the " +
				"attribute by that id.",
		},
		{
			Package:    securityAttributesPackage,
			SchemaType: "SecurityAttributeProjectUpdatePayload",
			Field:      "project",
			Category:   categoryRestated,
			Reason: "The whole Project the caller named by project_id, handed back by " +
				"Mutations::Security::Attributes::ProjectUpdate on success. What the call answers is how many " +
				"attributes it added and removed, which is published; the project's own fields are the project " +
				"domain's surface, published by gitlab_project.",
		},
		{
			Package:    securityAttributesPackage,
			SchemaType: "SecurityCategory",
			Field:      "securityAttributes",
			Category:   categoryNotThisResponse,
			Reason: "The category is selected so the caller knows which category the created or updated attribute " +
				"belongs to, and its attribute list is the category's own content: securitycategories publishes it " +
				"as security_attributes when a category is created or updated. Every attribute this call created " +
				"or updated is already in the response, and selecting the list would walk back from the category " +
				"to the attributes the response carries.",
		},
		{
			Package:    securityCategoriesPackage,
			SchemaType: "SecurityAttribute",
			Field:      "securityCategory",
			Category:   categoryRestated,
			Reason: "Each attribute in a category's security_attributes belongs to that category, which is the " +
				"object the response is: SecurityAttribute.securityCategory walks from the child back to the parent " +
				"the same payload carries, so selecting it would publish the category a second time under each of " +
				"its attributes.",
		},
		{
			Package:    securityCategoriesPackage,
			SchemaType: "SecurityCategoryDestroyPayload",
			Field:      "deletedCategoryGid",
			Category:   categoryRestated,
			Reason: "The global id of the deleted category, which is the id the caller sent: " +
				"Mutations::Security::Categories::Destroy finds the category by that argument and hands its id back " +
				"on success. The confirmation already names the category by that id, and the attributes deleted with " +
				"it, which the caller could not know, are selected and published as deleted_attribute_ids.",
		},
		{
			Package:    customEmojiPackage,
			SchemaType: "CustomEmoji",
			Field:      "userPermissions",
			Category:   categoryViewer,
			Reason: "CustomEmojiPermissions says whether the token's own user may create, delete or read the emoji, " +
				"which is a property of the caller and not of the emoji, and GitLab's REST entities carry no " +
				"equivalent. This server answers that question by attempting the action and reporting GitLab's " +
				"refusal, which is authoritative where a permissions snapshot taken at listing time can be stale.",
		},
	}
}

// ciCatalogDeclarations answers what the two catalog documents leave out: the
// user and the commit a version names, a rule's parsed condition, and the
// Enterprise field that lists the projects using a resource.
func ciCatalogDeclarations() []sentDeclaration {
	return []sentDeclaration{
		{
			Package:    ciCatalogPackage,
			SchemaType: "UserCore",
			Field:      declaredSegment,
			Category:   categoryNotThisResponse,
			Reason: userReferenceReason("the user who published a catalog version",
				"the id, username, name, web URL and avatar"),
		},
		{
			Package:    ciCatalogPackage,
			SchemaType: "Commit",
			Field:      declaredSegment,
			Category:   categoryNotThisResponse,
			Reason: referenceStub("commit a catalog version was released from", "repository.commit_get") +
				" The version carries the SHA, short id, title and web URL, which identify the commit.",
		},
		{
			Package:    ciCatalogPackage,
			SchemaType: "CiInputsRule",
			Field:      "conditionTree",
			Category:   categoryRecursiveShape,
			Reason: "conditionTree is GitLab's parse of the rule's if expression into a CiInputsCondition, whose " +
				"children are CiInputsConditions again, so a document could only select it to a depth fixed in " +
				"advance. GitLab builds it from the if expression for its own frontend to evaluate " +
				"(Types::Ci::Inputs::RuleType#condition_tree), and the expression, which is selected and published " +
				"as written, is the whole of what it says.",
		},
		{
			Package:    ciCatalogPackage,
			SchemaType: "CiCatalogResource",
			Field:      "projectComponentUsages",
			Category:   categoryTierAboveDomain,
			Reason: "CiCatalogResource.projectComponentUsages (the projects using the resource's components) is " +
				"defined in GitLab's Enterprise edition alone (ee/app/graphql/ee/types/ci/catalog/resource_type.rb " +
				"prepends it), and the catalog get is served on every tier, so a Community instance would refuse the " +
				"whole get document for naming it. On an Enterprise one its resolver " +
				"(ee/app/graphql/resolvers/ci/catalog/resources/project_component_usages_resolver.rb) raises a " +
				"resource-not-available error unless the resource's top-level namespace holds the Premium feature " +
				"ci_component_usages_in_projects, and answers null to anyone but a maintainer of the resource " +
				"project. It was also added in 18.11, past the get's 18.10 floor, and carries FieldCallCount limit 1, " +
				"so no listing can ask it of a page of resources.",
		},
	}
}

// branchRuleDeclarations answers what the branch rule documents leave out: the
// experiments, the one field newer than the Enterprise document's release, the
// recursive parent of a granted group, and the users an approval rule names.
func branchRuleDeclarations() []sentDeclaration {
	declarations := []sentDeclaration{
		{
			Package:    branchRulesPackage,
			SchemaType: "UserCore",
			Field:      declaredSegment,
			Category:   categoryNotThisResponse,
			Reason: userReferenceReason("a user an approval rule lets approve",
				"the identity GitLab's own access-level user carries (id, username, name, public email, avatar and "+
					"web URL and path), which is the shape every user a branch rule names is published in"),
		},
		{
			Package:    branchRulesPackage,
			SchemaType: "ApprovalProjectRule",
			Field:      "coverageMinimumThreshold",
			Category:   categoryNewerThanFloor,
			Reason: "coverageMinimumThreshold (the coverage below which a coverage-check rule requires approval) was " +
				"added in GitLab 19.2: GitLab's versioned GraphQL reference lists ApprovalProjectRule." +
				"coverageMinimumThreshold from 19.2 and not in 19.1. GitLab refuses a whole document that names a " +
				"field it does not have. Every other field the Enterprise branch rule document selects is served from " +
				"18.8 (the two warn-mode policy flags are the newest), and a release that refuses it is asked again " +
				"with the base document, which is served from 16.11 and asks for none of the security-policy flags. " +
				"Selecting the threshold would send every Premium and Ultimate instance from 18.8 to 19.1 to the base " +
				"document, losing the four flags there for one number.",
		},
		{
			Package:    branchRulesPackage,
			SchemaType: "AccessLevelGroup",
			Field:      "parent",
			Category:   categoryRecursiveShape,
			Reason: "parent is an AccessLevelGroup itself, so the chain above a granted group can only be selected to " +
				"a depth fixed in advance, and the group's web_url, which is published, already spells its whole " +
				"path. Selecting one level of it under the three grant lists also took the Enterprise document to a " +
				"complexity of 256 at a page of 100 on GitLab.com (2026-09-26), past the 250 GitLab allows an " +
				"authenticated caller, where without it the document scores 226.",
		},
		{
			Package:    branchRulesPackage,
			SchemaType: "BranchRule",
			Field:      "isGroupLevel",
			Category:   categoryExperiment,
			Reason:     branchRuleExperimentReason("BranchRule.isGroupLevel", "19.3", "whether the rule was created at the group level"),
		},
		{
			Package:    branchRulesPackage,
			SchemaType: "BranchRule",
			Field:      "squashOption",
			Category:   categoryExperiment,
			Reason: branchRuleExperimentReason("BranchRule.squashOption", "17.9",
				"the default squash behavior of merge requests into the matched branches"),
		},
		{
			Package:    branchRulesPackage,
			SchemaType: "BranchProtection",
			Field:      "isGroupLevel",
			Category:   categoryExperiment,
			Reason: branchRuleExperimentReason("BranchProtection.isGroupLevel", "18.3",
				"whether the protection was created at the group level"),
		},
	}
	for _, grant := range []string{"PushAccessLevel", "MergeAccessLevel", "UnprotectAccessLevel"} {
		declarations = append(declarations, sentDeclaration{
			Package:    branchRulesPackage,
			SchemaType: grant,
			Field:      "memberRole",
			Category:   categoryExperiment,
			Reason: branchRuleExperimentReason(grant+".memberRole", "19.2",
				"the custom role a grant names, resolved only while custom roles for protected branches are enabled"),
		})
	}
	return declarations
}
