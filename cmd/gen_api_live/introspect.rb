# Ask a booted GitLab what its REST API is, by asking the loaded application
# rather than by reading its source.
#
# Every other oracle in this repository about GitLab's REST API is a reading of
# text somebody else wrote: the OpenAPI document GitLab commits, or a scan of
# the Ruby that document is generated from. Both are downstream of the thing
# that decides what a request actually returns, which is the Rails application
# with its classes loaded. This script asks that.
#
# It answers three questions in one boot:
#
#   routes    - every endpoint Grape has mounted, with its method, its path,
#               the entity its `desc ... success/entity` annotation names, and
#               its declared params with type, requiredness and default.
#   entities  - every API::Entities class, with the fields it exposes, the
#               entity a field renders with, and the condition that gates it.
#   features  - GitLab's licensed feature table, evaluated.
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

require "json"

SCHEMA_VERSION = 1

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
  (start...[start + 12, lines.size].min).each do |index|
    text << lines[index]
    depth += lines[index].count("{([") - lines[index].count("})]")
    break if depth <= 0 && index > start
    break if depth.zero? && lines[index].strip.end_with?("end")
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
# A hash condition (`if: { admin: true }`) carries its own data. A block
# condition carries a Proc, whose source location is the only handle on what it
# tests, so the location and the text read back from it are both recorded: the
# location so a reader can go there, the text so a rule can classify without
# opening the image.
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
    entry["inverse"] = true if condition.instance_variable_get(:@inverse)

    block = condition.instance_variable_get(:@block)
    if block.respond_to?(:source_location) && block.source_location
      file, line = block.source_location
      entry["file"] = repo_relative(file)
      entry["line"] = line
      text = read_source(file, line)
      entry["text"] = text if text
    end

    hash = condition.instance_variable_get(:@hash)
    entry["hash"] = squeeze(hash.inspect) if hash

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

# FEATURE_LISTS maps each list to the tier it stands for, matching
# cmd/internal/apiexposes/features.go so that this record is a drop-in for the
# readers built on that one: STARTER counts as premium, and where a symbol
# appears under several lists the highest rank wins.
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
      rescue StandardError, NameError
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

entities = entities_document
routes = routes_document
features = features_document

puts JSON.generate(
  "schema_version" => SCHEMA_VERSION,
  "gitlab_version" => Gitlab::VERSION,
  "gitlab_revision" => (Gitlab.revision rescue nil),
  "entities" => entities,
  "routes" => routes,
  "features" => features,
)
