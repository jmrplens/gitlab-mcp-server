# Ask a booted GitLab what its REST API is, by asking the loaded application
# rather than by reading its source.
#
# Every other oracle in this repository about GitLab's REST API is a reading of
# text somebody else wrote: the OpenAPI document GitLab commits, or a scan of
# the Ruby that document is generated from. Both are downstream of the thing
# that decides what a request actually returns, which is the Rails application
# with its classes loaded. This script asks that.
#
# It answers five questions in one boot:
#
#   routes    - every endpoint Grape has mounted, with its method, its path,
#               the entity its `desc ... success/entity` annotation names, its
#               declared params with type, requiredness and default, and what
#               it demands of a fine-grained personal access token.
#   entities  - every API::Entities class, with the fields it exposes, the
#               entity a field renders with, and the condition that gates it.
#   features  - GitLab's licensed feature table, evaluated.
#   granular  - the permission vocabulary a fine-grained token is granted in:
#               every assignable permission with the raw permissions it
#               expands to, which assignable GitLab names for a raw one, and
#               what an anonymous caller may do on a public project and group.
#   graphql_authz - what each GraphQL object type, mutation and field demands
#               of a fine-grained token, and the shape of every object-typed
#               field, which decides what a denial does to an answer.
#
# Why evaluation beats parsing, concretely: GeoSiteStatus exposes its fields by
# iterating a constant assembled from two method calls, so its source says
# "expose the loop variable" and a scanner sees 26 fields where GitLab sends
# 606. ApplicationSetting splats a helper's attribute list, 81 against 680.
# Neither is a hole a better parser could close, because the names are not in
# the file: they are the value of a method that runs at load time.
#
# What evaluation does NOT give, and why the source is still read here: a
# condition is a Proc, and a Proc knows its source location but not its text.
# So for every block condition this reads back the lines it points at, from
# inside the image, and reports the condition as written. That is the one place
# this script touches source, and it touches the source of the exact instance
# it is describing.
#
# Run inside the container:
#
#   gitlab-rails runner introspect.rb
#
# It writes one JSON object to stdout and nothing else, so a caller can pipe it.

require "digest"
require "json"

# Version 2 marks a merged exposure. Version 1 spelled it exactly like a nested
# one, which read as the opposite of what GitLab sends.
#
# Version 3 records what a hash and a symbol condition test. Version 2 read a
# hash out of an instance variable grape-entity never sets and had nowhere to
# put a symbol, so both arrived as their kind alone and every field they gated
# read as unconditional.
#
# Version 4 records fine-grained authorization: a route's `authorization`, and
# the `granular` and `graphql_authz` blocks. In version 4 a route without an
# `authorization` declares nothing, so a fine-grained token is denied there,
# while in version 3 the same absence meant "not recorded"; a reader taking one
# for the other would deny every route, which is the silent inversion the
# version exists to stop.
SCHEMA_VERSION = 4

# ---------------------------------------------------------------------------
# Entities
# ---------------------------------------------------------------------------

# entity_classes finds every loaded Grape entity under API::Entities.
#
# ObjectSpace rather than walking the API::Entities namespace: an entity may be
# reopened, prepended into by an EE module, or defined under a nested module,
# and the object graph has all of them where a constant walk needs to know
# every shape in advance.
#
# The rescue is Exception rather than StandardError on purpose. GitLab has
# classes that override .name to raise NotImplementedError, which descends from
# ScriptError, and one of them aborts the whole enumeration if it is not caught
# here.
def entity_classes
  found = {}
  ObjectSpace.each_object(Class) do |klass|
    name =
      begin
        klass.name
      rescue Exception # rubocop:disable Lint/RescueException
        nil
      end
    next if name.nil? || !name.start_with?("API::Entities::")
    next unless klass < Grape::Entity

    found[name] = klass
  end
  found
end

# read_source returns the lines a Proc's source location points at, joined and
# squeezed onto one line, so a condition can be read as written.
#
# It reads forward until the brackets balance rather than taking one line: most
# lambdas are one line and some are not, and a bound stops a miscount from
# running to the end of the file.
def read_source(file, line)
  return nil unless file && line && File.readable?(file)

  lines = File.readlines(file)
  start = line - 1
  return nil if start.negative? || start >= lines.size

  text = +""
  depth = 0
  # Sliced rather than walked by index: the slice clamps at the end of the file
  # on its own, and the offset the enumerator gives is the only number the
  # balance test needs, which is whether this is still the first line.
  lines[start, 12].each_with_index do |source, offset|
    text << source
    depth += source.count("{([") - source.count("})]")
    break if depth <= 0 && offset.positive?
    break if depth.zero? && source.strip.end_with?("end")
  end
  squeeze(text)
end

def squeeze(text)
  text.strip.gsub(/\s+/, " ")[0, 400]
end

# repo_relative strips the image's install prefix so a path reads the way the
# same file reads in gitlab-org/gitlab.
def repo_relative(path)
  path.to_s.sub(%r{\A.*/gitlab-rails/}, "")
end

# conditions_of describes what gates an exposure.
#
# grape-entity has three kinds, and each carries what it tests differently. A
# symbol condition (`if: :with_custom_attributes`) carries the option the
# presenter must be given, and a hash condition (`if: { type: :full }`) the
# options and the values they must hold, both as data. A block condition
# carries a Proc, whose source location is the only handle on what it tests, so
# the location and the text read back from it are both recorded: the location
# so a reader can go there, the text so a rule can classify without opening the
# image.
#
# Each is read through the public reader its class declares (`inversed?`,
# `block`, `cond_hash`, `symbol`) and never through an instance variable. An
# instance variable that is not there reads as nil, which is how version 2 of
# this script asked for a `@hash` grape-entity never sets and recorded all 41
# hash and symbol conditions of 19.3.1-ee as their kind alone; a reader that
# grape-entity renames raises instead, and the run fails where somebody sees it.
#
# A kind none of the three branches knows is recorded as its kind and nothing
# else, and gen_api_live refuses to write a record holding one: the field it
# gates would read as gated by something nobody can name.
def conditions_of(exposure)
  conditions =
    begin
      exposure.conditions
    rescue StandardError
      []
    end
  return nil if conditions.nil? || conditions.empty?

  conditions.map do |condition|
    entry = { "kind" => condition.class.name.split("::").last }
    entry["inverse"] = true if condition.inversed?

    case condition
    when Grape::Entity::Condition::BlockCondition
      block = condition.block
      if block.respond_to?(:source_location) && block.source_location
        file, line = block.source_location
        entry["file"] = repo_relative(file)
        entry["line"] = line
        text = read_source(file, line)
        entry["text"] = text if text
      end
    when Grape::Entity::Condition::HashCondition
      entry["hash"] = squeeze(condition.cond_hash.inspect)
    when Grape::Entity::Condition::SymbolCondition
      entry["symbol"] = condition.symbol.to_s
    else
      # A kind none of the branches above reads keeps its kind alone, which is
      # what makes gen_api_live refuse the record rather than read the field
      # it gates as always sent.
      nil
    end

    entry
  end
end

# using_of names the entity a field renders with, which is what makes the
# record walkable: a response is a tree of entities and this is the edge.
def using_of(exposure)
  name =
    begin
      exposure.respond_to?(:using_class_name) ? exposure.using_class_name : nil
    rescue StandardError
      nil
    end
  return nil if name.nil?

  name.to_s
end

# merge_of reports whether an exposure is merged into the object around it
# rather than nested under its key. `expose :user, merge: true, using: UserBasic`
# on a member sends the user's own keys on the member and no `user` key, so an
# exposure read without this says the opposite of what GitLab sends.
#
# grape-entity keeps the flag on the exposure as for_merge, and the option is
# read as a fallback so a version that drops the reader is not silently read as
# no merges anywhere.
def merge_of(exposure)
  value =
    begin
      if exposure.respond_to?(:for_merge)
        exposure.for_merge
      elsif exposure.respond_to?(:options)
        exposure.options[:merge]
      end
    rescue StandardError
      nil
    end

  !value.nil? && value != false
end

def entities_document
  document = {}
  entity_classes.sort.each do |name, klass|
    exposures =
      begin
        klass.root_exposures
      rescue StandardError => error
        document[name] = { "error" => error.class.name }
        next
      end

    fields = exposures.map do |exposure|
      field = { "name" => exposure.key.to_s }
      attribute = exposure.attribute.to_s
      # `as:` renames a field, and both halves matter: the key is what GitLab
      # sends and the attribute is what a reader greps the source for.
      field["attribute"] = attribute if attribute != exposure.key.to_s
      using = using_of(exposure)
      field["using"] = using if using
      field["merge"] = true if merge_of(exposure)
      conditions = conditions_of(exposure)
      field["conditions"] = conditions if conditions
      field
    end

    document[name] = { "fields" => fields }
  end
  document
end

# ---------------------------------------------------------------------------
# Routes
# ---------------------------------------------------------------------------

# models_of collects the entity classes an annotation names, in any of the
# three shapes Grape stores one in.
#
# Measured on 19.3.1-ee, the `:entity` value is a Class 1087 times, a Hash 790
# times and an Array 54 times. A Hash is a status annotation,
# `{code: 200, model: API::Entities::X, example: {…}}`, and only 323 of them
# carry a model at all: the rest are `{code: 200, message: "200 OK"}` and name
# no entity. An Array holds several such annotations, one per status.
#
# Reading only the Class shape and stringifying the rest is how a first cut of
# this script reported 1465 entities where 1425 exist: 54 stringified arrays
# that named nothing, and 15 models nested in an array that it never looked
# into.
def models_of(value)
  case value
  when Class then [value.name]
  when Hash then value[:model].is_a?(Class) ? [value[:model].name] : []
  when Array then value.flat_map { |element| models_of(element) }
  else []
  end
end

# entity_of names the entity a route renders.
#
# This is the same annotation GitLab's own OpenAPI generator reads, so the
# record says what that document would say without the document in between. It
# can be wrong about what the endpoint really presents and that is not this
# script's to correct: GET /keys is annotated APIEntitiesUserWithAdmin and
# serves an SSH key with a user under it. Recording the annotation faithfully
# is what lets an audit hold it against something else and notice.
#
# No route in 19.3.1-ee names more than one distinct model, so a single name is
# the shape rather than a simplification; a second one would be dropped, which
# is why it is counted instead.
def entity_of(description)
  return [nil, 0] if description.nil?

  names = (models_of(description[:entity]) + models_of(description[:success])).uniq
  [names.first, names.size]
end

# params_of records what an endpoint declares it accepts. Grape keeps type,
# requiredness, default and the documented example, all of which say more than
# a bare parameter name: an audit can ask whether a required param is ever
# sent, or whether a name we send is one the endpoint declares at all.
def params_of(route)
  declared =
    begin
      route.params
    rescue StandardError
      nil
    end
  return nil if declared.nil? || declared.empty?

  declared.each_with_object({}) do |(name, spec), out|
    entry = {}
    if spec.is_a?(Hash)
      entry["required"] = true if spec[:required]
      entry["type"] = spec[:type].to_s if spec[:type]
      entry["default"] = spec[:default].inspect[0, 120] unless spec[:default].nil?
      entry["desc"] = squeeze(spec[:desc].to_s)[0, 200] if spec[:desc]
    end
    out[name.to_s] = entry
  end
end

# The keys of `route_setting :authorization` that decide what a fine-grained
# personal access token may do, as lib/api/helpers.rb reads them
# (authorize_granular_token_scopes! and the methods below it).
FINE_GRAINED_AUTHORIZATION_KEYS = %i[
  permissions boundary_type boundary_param boundaries boundary
  additional_scopes skip_granular_token_authorization todo assignable_when
].freeze

# The keys the same hash carries for CI job tokens. They answer a different
# question about a different credential, so they are known and left out rather
# than recorded; every other key is recorded by name as unknown, so a new
# option is seen rather than dropped.
JOB_TOKEN_AUTHORIZATION_KEYS = %i[
  job_token_policies skip_job_token_policies allow_public_access_for_enabled_project_features
].freeze

# What one boundary alternative, or one additional scope, may carry.
BOUNDARY_KEYS = %i[boundary_type boundary_param boundary].freeze
ADDITIONAL_SCOPE_KEYS = (BOUNDARY_KEYS + %i[permissions]).freeze

def names_of(value)
  Array(value).map(&:to_s)
end

# unknown_keys_of names the keys of a hash nothing here reads, sorted.
def unknown_keys_of(hash, known)
  (hash.keys.map(&:to_s) - known.map(&:to_s)).sort
end

# callable_of records a boundary given as a callable, which Grape evaluates per
# request, the way a block condition is recorded: located, and quoted from the
# image. What it returns is only known at run time, so the record says where to
# read it rather than guessing.
def callable_of(value)
  return { "callable" => false, "text" => squeeze(value.inspect) } unless value.respond_to?(:call)

  entry = { "callable" => true }
  if value.respond_to?(:source_location) && value.source_location
    file, line = value.source_location
    entry["file"] = repo_relative(file)
    entry["line"] = line
    text = read_source(file, line)
    entry["text"] = text if text
  end
  entry
end

# boundary_of records one boundary alternative or additional scope. Something
# that is not a hash at all is recorded as an unknown key naming its class,
# which is a shape no reader here understands and the gate refuses.
def boundary_of(spec, known = BOUNDARY_KEYS)
  return { "unknown_keys" => ["(#{spec.class.name})"] } unless spec.is_a?(Hash)

  out = {}
  out["boundary_type"] = spec[:boundary_type].to_s if spec[:boundary_type]
  out["boundary_param"] = spec[:boundary_param].to_s if spec[:boundary_param]
  out["boundary"] = callable_of(spec[:boundary]) unless spec[:boundary].nil?
  extra = unknown_keys_of(spec, known)
  out["unknown_keys"] = extra unless extra.empty?
  out
end

# authorization_of records what a route demands of a fine-grained token.
#
# Every key is read as lib/api/helpers.rb reads it: permissions are raw
# permission names; a callable boundary wins over boundaries and boundary_type
# when GitLab resolves the request, which a reader decides, so all three are
# recorded as declared; each additional scope must pass on its own; any skip
# reason disables the check. nil means the route declares nothing a
# fine-grained token can satisfy, which GitLab answers with "This operation
# doesn't support fine-grained personal access tokens".
def authorization_of(settings)
  auth = settings && settings[:authorization]
  return nil unless auth.is_a?(Hash)

  out = {}
  permissions = names_of(auth[:permissions])
  out["permissions"] = permissions unless permissions.empty?
  out["boundary_type"] = auth[:boundary_type].to_s if auth[:boundary_type]
  out["boundary_param"] = auth[:boundary_param].to_s if auth[:boundary_param]
  out["boundaries"] = Array(auth[:boundaries]).map { |spec| boundary_of(spec) } if auth[:boundaries]
  out["boundary"] = callable_of(auth[:boundary]) unless auth[:boundary].nil?
  if auth[:additional_scopes]
    out["additional_scopes"] = Array(auth[:additional_scopes]).map do |spec|
      scope = boundary_of(spec, ADDITIONAL_SCOPE_KEYS)
      scope_permissions = spec.is_a?(Hash) ? names_of(spec[:permissions]) : []
      scope["permissions"] = scope_permissions unless scope_permissions.empty?
      scope
    end
  end
  skip = auth[:skip_granular_token_authorization]
  out["skip"] = skip.to_s if skip
  out["todo"] = auth[:todo].to_s if auth[:todo].present?
  conditions = names_of(auth[:assignable_when])
  out["assignable_when"] = conditions unless conditions.empty?
  extra = unknown_keys_of(auth, FINE_GRAINED_AUTHORIZATION_KEYS + JOB_TOKEN_AUTHORIZATION_KEYS)
  out["unknown_keys"] = extra unless extra.empty?
  out.empty? ? nil : out
end

def routes_document
  ::API::API.routes.map do |route|
    settings =
      begin
        route.settings
      rescue StandardError
        nil
      end
    description = settings && (settings[:description] || settings[:desc])

    entry = {
      "method" => route.request_method.to_s,
      # origin rather than path: path carries Grape's (.:format) suffix, which
      # is a routing detail and not part of any endpoint anyone calls.
      "path" => route.origin.to_s,
    }
    entity, model_count = entity_of(description)
    entry["entity"] = entity if entity
    # Recorded rather than dropped: the single name above is the shape today,
    # and a route that grew a second model would otherwise lose it silently.
    entry["models"] = model_count if model_count > 1
    detail = description && description[:description]
    entry["summary"] = squeeze(detail.to_s)[0, 200] if detail
    params = params_of(route)
    entry["params"] = params if params
    authorization = authorization_of(settings)
    entry["authorization"] = authorization if authorization
    entry
  end
end

# ---------------------------------------------------------------------------
# Licensed features
# ---------------------------------------------------------------------------

# features_document maps every licensed feature symbol to the tier that unlocks
# it, read from the loaded table rather than parsed.
#
# The parsed version cannot read the lists the table builds by concatenation
# (ALL_PREMIUM_FEATURES and the like), because they are not array literals in
# the source. Here they are just values.
TIER_RANK = { "global" => 1, "premium" => 2, "ultimate" => 3 }.freeze

# FEATURE_LISTS maps each list to the tier it stands for, keeping the
# resolution the scanned record used before this one replaced it: STARTER
# counts as premium, and where a symbol appears under several lists the highest
# rank wins.
#
# Deliberately not re-litigated here. The tier tags across internal/tools and
# every rule that reads them were written against that resolution, and changing
# it silently underneath them would be a worse defect than either answer.
FEATURE_LISTS = {
  :GLOBAL_FEATURES => "global",
  :STARTER_FEATURES => "premium",
  :PREMIUM_FEATURES => "premium",
  :ULTIMATE_FEATURES => "ultimate",
}.freeze

def features_document
  table = defined?(::GitlabSubscriptions::Features) ? ::GitlabSubscriptions::Features : nil
  return {} if table.nil?

  tiers = {}
  FEATURE_LISTS.each do |constant, tier|
    list =
      begin
        table.const_get(constant)
      # NameError alone, and not StandardError beside it: NameError descends
      # from StandardError, so naming both rescued nothing the first did not
      # and only widened what is swallowed. A list this release does not
      # declare is the one failure worth passing over here.
      rescue NameError
        []
      end
    list.each do |feature|
      name = feature.to_s
      current = tiers[name]
      tiers[name] = tier if current.nil? || TIER_RANK[tier] > TIER_RANK[current]
    end
  end
  tiers
end

# ---------------------------------------------------------------------------
# The fine-grained permission vocabulary
# ---------------------------------------------------------------------------

ASSIGNABLE = ::Authz::PermissionGroups::Assignable

# assignable_of records one assignable permission, the name a user grants.
#
# display is "Resource: Action" exactly as GitLab writes it in a refusal
# (Authz::Tokens::AuthorizeGranularScopesService#access_denied_error), so a
# reader quotes GitLab's own words instead of re-deriving Rails' titleize.
def assignable_of(assignable)
  entry = {
    "name" => assignable.name.to_s,
    "category" => assignable.category.to_s,
    "category_name" => assignable.category_name.to_s,
    "resource" => assignable.resource.to_s,
    "resource_name" => assignable.resource_name.to_s,
    "action" => assignable.action.to_s,
    "display" => "#{assignable.resource_name}: #{assignable.action.titleize}",
    "boundaries" => names_of(assignable.boundaries),
    "permissions" => names_of(assignable.permissions),
    "available_for" => names_of(assignable.available_for),
  }
  entry["deprecated"] = true if assignable.deprecated?
  conditions = assignable.assignable_when.map do |condition|
    named = { "condition" => condition[:condition].to_s }
    boundaries = names_of(condition[:boundaries])
    named["boundaries"] = boundaries unless boundaries.empty?
    named
  end
  entry["assignable_when"] = conditions unless conditions.empty?
  entry
end

# available_to_a_token? is the assignable a user can actually grant: GitLab's
# own available_for_permission rejects deprecated definitions and ignores
# available_for, so it could name an assignable no token can hold.
def available_to_a_token?(assignable)
  !assignable.deprecated? && assignable.available_for?(:granular_access_token)
end

# raw_to_assignable maps every raw permission some assignable expands to onto
# the assignable GitLab names for it, in two readings.
#
# first is Assignable.for_permission(raw).first, which searches every
# definition, deprecated ones included, in file-path order, and is the name a
# refusal prints. first_available is the first in the same order a token can be
# granted, and is the name a person should be told to grant.
def raw_to_assignable
  ASSIGNABLE.all_permissions.map(&:to_s).sort.to_h do |raw|
    matches = ASSIGNABLE.for_permission(raw)
    entry = { "first" => matches.first.name.to_s }
    available = matches.find { |assignable| available_to_a_token?(assignable) }
    entry["first_available"] = available.name.to_s if available
    [raw, entry]
  end
end

# flag_default_enabled is whether fine-grained tokens are on unless an
# administrator turns them off, read from the flag's own definition.
def flag_default_enabled
  definition = ::Feature::Definition.get(:granular_personal_access_tokens)
  definition&.default_enabled
end

def granular_document
  document = {
    "assignable" => ASSIGNABLE.definitions.map { |assignable| assignable_of(assignable) },
    "raw_permissions" => ::Authz::Permission.all.keys.map(&:to_s).sort,
    "raw_to_assignable" => raw_to_assignable,
  }
  enabled = flag_default_enabled
  document["flag_default_enabled"] = enabled unless enabled.nil?
  document
end

# ---------------------------------------------------------------------------
# What an anonymous caller may do on a public project and group
# ---------------------------------------------------------------------------

# PUBLIC_ANONYMOUS_SOURCE names how the subjects were built: "unsaved" records
# that were never written, rather than a public project and group created
# inside the container ("persisted") or the end-to-end fixture's ("fixture").
PUBLIC_ANONYMOUS_SOURCE = "unsaved"

# AllLicensed makes every licensed feature available for the rest of this run.
#
# The anonymous set is the widest any public project or group serves, and on a
# licensed instance that includes the reads a licence unlocks (an epic on a
# public group, a requirement on a public project). This process is a
# throwaway runner inside a throwaway container, so answering yes for every
# feature here changes nothing anybody serves.
module AllLicensed
  def feature_available?(*)
    true
  end
end

module AllLicensedNamespace
  def licensed_feature_available?(*)
    true
  end
end

def license_everything
  ::License.singleton_class.prepend(AllLicensed) if defined?(::License)
  ::Namespace.prepend(AllLicensedNamespace) if ::Namespace.method_defined?(:licensed_feature_available?)
  ::Project.prepend(AllLicensedNamespace) if ::Project.method_defined?(:licensed_feature_available?)
end

# public_subjects builds a public group and a public project in it, with every
# project feature enabled, and saves neither. Enabled is the widest level a
# public project serves, so the set read from them over-approximates any public
# project whose settings serve less, which on REST is GitLab's own 403.
def public_subjects
  organization = ::Organizations::Organization.first
  group = ::Group.new(name: "public", path: "public", organization: organization,
    visibility_level: ::Gitlab::VisibilityLevel::PUBLIC)
  project = ::Project.new(name: "public", path: "public", namespace: group, organization: organization,
    visibility_level: ::Gitlab::VisibilityLevel::PUBLIC)
  feature = project.build_project_feature
  ::ProjectFeature.available_features.each do |name|
    setter = "#{name}_access_level="
    feature.public_send(setter, ::Featurable::ENABLED) if feature.respond_to?(setter)
  end
  { "project" => project, "group" => group }
end

# public_anonymous_document evaluates GitLab's anonymous policy for every raw
# permission an assignable expands to, which is the only set the public bypass
# of Authz::BoundaryPolicy is defined over.
#
# It evaluates rather than reads config/authz/roles/public_anonymous.yml
# because the role file is a lower bound: ProjectPolicy derives read_issue_link,
# read_work_item, read_design and read_attestation from other abilities on a
# public project, and the file names none of them. A permission that raises is
# not guessed at: the block is left out and the error recorded, and a reader
# then has no public set to rely on.
def public_anonymous_document
  license_everything
  permissions = ASSIGNABLE.all_permissions.map(&:to_sym).uniq.sort
  sets = public_subjects.transform_values do |subject|
    permissions.select { |permission| ::Users::Anonymous.can?(permission, subject) }.map(&:to_s)
  end
  { "public_anonymous" => { "source" => PUBLIC_ANONYMOUS_SOURCE, "licensed" => true }.merge(sets) }
rescue StandardError => error
  { "public_anonymous_error" => squeeze("#{error.class}: #{error.message}") }
end

# ---------------------------------------------------------------------------
# GraphQL authorization
# ---------------------------------------------------------------------------

GRANULAR_SCOPE = ::Directives::Authz::GranularScope

# The arguments of Directives::Authz::GranularScope. Any other is recorded by
# name as unknown, so an argument a later release adds is seen.
GRANULAR_DIRECTIVE_ARGUMENTS = %w[
  permissions boundary_type boundary boundary_argument requirement_group assignable_when skip_reason
].freeze

def directive_of(directive)
  out = {}
  unknown = []
  directive.arguments.keyword_arguments.each do |key, value|
    next if value.nil?

    name = key.to_s
    if GRANULAR_DIRECTIVE_ARGUMENTS.include?(name)
      out[name] = value.is_a?(Array) ? value.map(&:to_s) : value.to_s
    else
      unknown << name
    end
  end
  out["unknown_keys"] = unknown.sort unless unknown.empty?
  out
end

# granular_of lists the GranularScope directives a type, field or mutation
# class carries, inherited ones included, in declaration order: GitLab reads
# the permissions of a requirement group from its first directive only.
def granular_of(member)
  return [] unless member.respond_to?(:directives)

  member.directives.select { |directive| directive.is_a?(GRANULAR_SCOPE) }.map { |directive| directive_of(directive) }
end

# calls_super? reads whether an override of authorized? hands the decision back
# to the class above it, from the compiled method rather than its text.
def calls_super?(method)
  iseq = RubyVM::InstructionSequence.of(method)
  !iseq.nil? && iseq.disasm.include?("invokesuper")
rescue StandardError
  false
end

def owner_name(owner)
  attached = owner.respond_to?(:attached_object) ? (owner.attached_object rescue nil) : nil
  name = attached.respond_to?(:name) ? attached.name : nil
  (name || owner.name || owner.to_s).to_s
end

# object_fields_of records the signature of every field whose type is an
# object, an interface or a union, which is what decides what a denial there
# does to the answer: a nullable field comes back null, a non-null one carries
# its null upward, and a list or connection may have the item removed.
def object_fields_of(type)
  return {} unless type.respond_to?(:fields)

  type.fields.sort.each_with_object({}) do |(name, field), out|
    kind = field.type.unwrap.kind
    next unless kind.object? || kind.interface? || kind.union?

    entry = { "type" => field.type.to_type_signature }
    entry["connection"] = true if field.connection?
    out[name] = entry
  end
end

# type_of describes one object type.
#
# enforced is whether GitLab's granular check runs on an object of the type:
# Types::BaseObject.authorized? runs it, so a class outside BaseObject never
# does, and one that overrides authorized? does only when the override calls
# super. authorized_by names the override. abilities are what decides, beside
# a directive, whether a list of the type is redacted rather than nulled.
def type_of(type)
  entry = {}
  entry["class"] = type.name.to_s if type.name
  runs_base = type.respond_to?(:granular_scope_authorization)
  owner = type.method(:authorized?).owner
  overridden = runs_base && owner != ::Types::BaseObject.singleton_class
  entry["authorized_by"] = owner_name(owner) if overridden
  entry["enforced"] = runs_base && (!overridden || calls_super?(type.method(:authorized?)))
  authorization = type.respond_to?(:authorization) ? type.authorization : nil
  abilities = authorization.respond_to?(:abilities) ? names_of(authorization.abilities) : []
  entry["abilities"] = abilities unless abilities.empty?
  granular = granular_of(type)
  entry["granular"] = granular unless granular.empty?
  fields = object_fields_of(type)
  entry["object_fields"] = fields unless fields.empty?
  entry
end

# mutation_class_of resolves the class behind a Mutation field, the way
# GitLab's own permission tasks do, and answers nil for a field no
# Mutations::BaseMutation backs.
def mutation_class_of(field)
  resolver = field.respond_to?(:resolver_class) ? field.resolver_class : nil
  resolver ||= field.respond_to?(:resolver) ? field.resolver : nil
  resolver ||= field.respond_to?(:mutation) ? field.mutation : nil
  resolver if resolver.is_a?(Class) && resolver < ::Mutations::BaseMutation
end

# mutation_directives_of reads a mutation's directives from its field first
# and its class second, which is where GitLab looks.
def mutation_directives_of(field, resolver)
  granular = granular_of(field)
  granular.empty? ? granular_of(resolver) : granular
end

# todo_rule_type? is the type half of the rule that generates GitLab's
# authorization_todo.txt (lib/tasks/gitlab/permissions/graphql/schema_directives.rb).
def todo_rule_type?(name, type)
  return false if name.start_with?("__")
  return false if %w[Mutation Query Subscription].include?(name)

  type.kind.object? && !name.end_with?("Payload", "Connection", "Edge")
end

AUTHORIZATION_TODO_FILE = "config/authz/graphql/authorization_todo.txt"

# todo_document reads the list GitLab keeps of the types and mutations it has
# not declared yet, when the image ships it, so the set computed below can be
# held to it.
def todo_document
  path = Rails.root.join(AUTHORIZATION_TODO_FILE)
  return nil unless File.exist?(path)

  content = File.read(path)
  out = { "digest" => "sha256:#{Digest::SHA256.hexdigest(content)}", "types" => [], "mutations" => [] }
  unknown = []
  content.each_line do |line|
    entry = line.strip
    next if entry.empty? || entry.start_with?("#")

    kind, name = entry.split(":", 2)
    case kind
    when "type" then out["types"] << name
    when "mutation" then out["mutations"] << name
    else unknown << entry
    end
  end
  out["types"].sort!
  out["mutations"].sort!
  out["unknown_entries"] = unknown.sort unless unknown.empty?
  out
end

def graphql_authz_document
  types = {}
  abstract = {}
  fields = {}
  rule_types = []
  GitlabSchema.types.sort.each do |name, type|
    next if name.start_with?("__")

    kind = type.kind
    if kind.object?
      types[name] = type_of(type)
      rule_types << name if todo_rule_type?(name, type) && type.directives.none? { |d| d.is_a?(GRANULAR_SCOPE) }
    elsif kind.union? || kind.interface?
      abstract[name] = {
        "kind" => kind.union? ? "union" : "interface",
        "possible_types" => GitlabSchema.possible_types(type).map(&:graphql_name).sort,
      }
    end
    next if name == "Mutation" || !type.respond_to?(:fields)

    type.fields.sort.each do |field_name, field|
      granular = granular_of(field)
      fields["#{name}.#{field_name}"] = granular unless granular.empty?
    end
  end

  mutations = {}
  rule_mutations = []
  GitlabSchema.types["Mutation"].fields.sort.each do |field_name, field|
    resolver = mutation_class_of(field)
    next unless resolver

    granular = mutation_directives_of(field, resolver)
    entry = {
      "name" => resolver.graphql_name.to_s,
      "class" => resolver.name.to_s,
      "payload" => resolver.payload_type.graphql_name.to_s,
    }
    entry["granular"] = granular unless granular.empty?
    mutations[field_name] = entry
    rule_mutations << resolver.graphql_name.to_s if granular.empty?
  end

  document = {
    "types" => types,
    "abstract" => abstract,
    "mutations" => mutations,
    "fields" => fields,
    "undeclared_by_todo_rule" => { "types" => rule_types.sort, "mutations" => rule_mutations.sort },
  }
  todo = todo_document
  document["todo"] = todo if todo
  document
end

# ---------------------------------------------------------------------------

entities = entities_document
routes = routes_document
features = features_document
granular = granular_document
graphql_authz = graphql_authz_document
# Last, because it makes every licensed feature available for the rest of the
# process, and nothing read before it may see that.
granular.merge!(public_anonymous_document)

puts JSON.generate(
  "schema_version" => SCHEMA_VERSION,
  "gitlab_version" => Gitlab::VERSION,
  "gitlab_revision" => (Gitlab.revision rescue nil),
  "entities" => entities,
  "routes" => routes,
  "features" => features,
  "granular" => granular,
  "graphql_authz" => graphql_authz,
)
