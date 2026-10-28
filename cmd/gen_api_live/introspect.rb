# Copyright (c) 2026 José M. Requena Plens
# SPDX-License-Identifier: MIT

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
#   keyset    - for every route, where the code it reaches calls a keyset
#               paginator and which parameter its next page is asked by, and for
#               each of GitLab's keyset paginators the parameter its next link
#               names, evaluated rather than read.
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
require "set"

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
#
# Version 5 records keyset pagination: a route's `keyset` and the
# `keyset_paginators` block. In version 5 a route without a `keyset` reaches no
# keyset paginator, so GitLab ignores pagination=keyset there and answers an
# offset page, while in version 4 the same absence meant "not recorded"; a
# reader taking one for the other would call every keyset route an offset one.
SCHEMA_VERSION = 5

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

# default_of renders a declared default the same way on every boot of one
# image. Two kinds of default are a value of the boot rather than of the
# route: a lambda, which Grape calls on each request and whose inspect names
# the address the object was given, is recorded by where it is written; and a
# time or a date, which the API computes when it loads, so its value is the
# moment the image booted, is recorded by its kind. Every other default is its
# inspect, as GitLab wrote it.
def default_of(default)
  if default.is_a?(Proc)
    file, line = default.source_location
    file ? "(computed per request, #{repo_relative(file)}:#{line})" : "(computed per request)"
  elsif default.acts_like?(:time) || default.acts_like?(:date)
    "(computed when the API loads)"
  else
    default.inspect[0, 120]
  end
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
      entry["default"] = default_of(spec[:default]) unless spec[:default].nil?
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

# primary_requirement_of reads a route's own requirement: the permissions it
# names and the boundary they are held at, in each of the three ways a route
# may declare one.
def primary_requirement_of(auth)
  out = {}
  permissions = names_of(auth[:permissions])
  out["permissions"] = permissions unless permissions.empty?
  out["boundary_type"] = auth[:boundary_type].to_s if auth[:boundary_type]
  out["boundary_param"] = auth[:boundary_param].to_s if auth[:boundary_param]
  out["boundaries"] = Array(auth[:boundaries]).map { |spec| boundary_of(spec) } if auth[:boundaries]
  out["boundary"] = callable_of(auth[:boundary]) unless auth[:boundary].nil?
  out
end

# additional_scope_of reads one additional scope: a boundary, read as a
# route's own is, and the permissions that must pass at it.
def additional_scope_of(spec)
  scope = boundary_of(spec, ADDITIONAL_SCOPE_KEYS)
  scope_permissions = spec.is_a?(Hash) ? names_of(spec[:permissions]) : []
  scope["permissions"] = scope_permissions unless scope_permissions.empty?
  scope
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

  out = primary_requirement_of(auth)
  if auth[:additional_scopes]
    out["additional_scopes"] = Array(auth[:additional_scopes]).map { |spec| additional_scope_of(spec) }
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

def routes_document(paginators)
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
    keyset = keyset_of(route, paginators)
    entry["keyset"] = keyset if keyset
    entry
  end
end

# ---------------------------------------------------------------------------
# Keyset pagination
# ---------------------------------------------------------------------------

# GitLab reads pagination=keyset without declaring it: of the 53 routes whose
# code reaches a keyset paginator at 19.5.0, four declare `pagination` and 22
# declare no cursor either, because the helper or the pager that serves the
# page reads both from params on its own. So a route's declaration cannot say
# whether it pages by keyset, and this section asks the code instead: it walks
# what each route's block reaches through the helpers it is served with, and
# records every place that calls a keyset paginator. That is where a keyset
# page can be served, not a promise that one is: see paginate_with_strategies
# below.
#
# Five shapes call one, each recorded under its own name:
#
#   paginate_with_strategies - the helper (lib/api/helpers/pagination_strategies.rb)
#       that pages by keyset when pagination=keyset: CursorPager, asked by
#       `cursor`, when the relation's model declares keyset orderings, and
#       Keyset::Pager, asked by `id_after` and `id_before`, when the call passes
#       `use_cursor: false` or the model declares none; anything but Project
#       ordered by id then answers 405. The model is the relation's, built when
#       a request is served, and the walk reads code, so a site is recorded
#       with CursorPager's cursor unless the call passes `use_cursor: false`:
#       on a model declaring no keyset orderings, PersonalAccessToken on the
#       service account token lists among them, GitLab answers every keyset
#       request there with 405 and never reads that cursor.
#   gitaly_keyset_pager - Gitlab::Pagination::GitalyKeysetPager, asked by
#       `page_token`, for branches, tags, the tree and commits.
#   gitaly_pagination - a finder executed with `gitaly_pagination: true`,
#       which pages through Gitaly by the same `page_token` without the pager.
#   keyset_header_builder - a route writing its own keyset next link or cursor
#       headers through Gitlab::Pagination::Keyset::HeaderBuilder.
#   keyset_paginate - a relation paged with `keyset_paginate(cursor: ...)`.
#
# The walk follows calls on the endpoint itself, which is what a helper call
# is, and never calls on another object: a finder or a service that pages by
# keyset on its own, and writes no next link through the route, is not seen.
# That is the gap the -check rule over declared cursors exists for. A method is
# walked once per route, so a site reached along two paths is recorded along
# the first.

# ISEQ_FORMAT is the marker an instruction sequence's array form starts with,
# which is how a block nested in an operand is told from any other array.
ISEQ_FORMAT = "YARVInstructionSequence/SimpleDataFormat"

# VM_CALL_FCALL is the call flag Ruby sets on a call with no receiver, a call
# on self: a helper call, as opposed to a call on a finder or a relation.
VM_CALL_FCALL = 1 << 2

# NON_POSITIONAL_KEYS are the keys a next link carries beside its cursor: the
# page size and the order, which repeat the request rather than position it.
NON_POSITIONAL_KEYS = %w[per_page page sort order_by pagination].freeze

# CursorProbe is the request context a paginator is handed when it is asked,
# below, which parameter its next link names: it answers params as a request
# would and keeps the headers the paginator writes.
class CursorProbe
  attr_reader :params, :headers

  def initialize(params)
    @params = ActiveSupport::HashWithIndifferentAccess.new(params)
    @headers = {}
  end

  def header(name, value)
    @headers[name] = value
  end

  def request
    Struct.new(:url).new("http://probe.invalid/api/v4/probe")
  end
end

# MissingNextLinkError is what a probe raises when the paginator it asked
# wrote no next link, so the run fails where somebody sees it rather than
# recording a paginator without the parameter its next page is asked by.
class MissingNextLinkError < StandardError; end

# next_link_of is the query of the next link a probe was given.
def next_link_of(probe)
  link = probe.headers["Link"].to_s[/<([^>]+)>;\s*rel="next"/, 1]
  raise MissingNextLinkError, "the paginator wrote no next link: #{probe.headers.inspect}" unless link

  Rack::Utils.parse_query(URI.parse(link).query)
end

# cursor_of names the parameters a next link adds to the request it answers,
# which is what the next page is asked by.
def cursor_of(probe)
  next_link_of(probe).keys - probe.params.keys.map(&:to_s)
end

# keyset_paginators_document asks each of GitLab's keyset paginators which
# parameter its next link names, by having it write one.
#
# Evaluated rather than written down here, for the reason this script exists:
# the parameter is a value of GitLab's code, and a table copied from it would
# keep the old name after a release renamed it. Each probe goes through the
# method GitLab itself calls once a page is served, so one that raises fails
# the run where somebody sees it. cursor_headers is the parameter the
# X-NEXT-CURSOR header is sent back as, read off the same CursorPager answer,
# which writes both.
def keyset_paginators_document
  cursor_probe = CursorProbe.new(per_page: 1)
  ::Gitlab::Pagination::Keyset::CursorBasedRequestContext.new(cursor_probe).apply_headers("probe-next", nil)
  next_cursor = cursor_probe.headers["X-NEXT-CURSOR"]
  cursor_headers = next_link_of(cursor_probe).select { |_name, value| value == next_cursor }.keys

  pager = %w[desc asc].flat_map do |sort|
    probe = CursorProbe.new(per_page: 1, order_by: "id", sort: sort)
    context = ::Gitlab::Pagination::Keyset::RequestContext.new(probe)
    context.apply_headers(context.page.next(id: 1))
    cursor_of(probe)
  end

  gitaly_probe = CursorProbe.new(per_page: 1)
  ::Gitlab::Pagination::GitalyKeysetPager.new(gitaly_probe, nil).send(:apply_headers, [nil], "probe-next")

  {
    "cursor_pager" => cursor_of(cursor_probe).sort,
    "pager" => pager.uniq.sort,
    "gitaly_keyset_pager" => cursor_of(gitaly_probe).sort,
    "cursor_headers" => cursor_headers.sort,
  }
end

def iseq_array?(value)
  value.is_a?(Array) && value.first == ISEQ_FORMAT
end

# gitlab_method? is whether a method is GitLab's own code rather than a gem's,
# which bounds the walk: Grape's present or ActiveSupport's helpers serve no
# keyset page, and walking them would multiply the run for nothing.
def gitlab_method?(method)
  file, = method.source_location
  return false unless file

  file.start_with?(Rails.root.to_s + "/") && !file.include?("/vendor/") && !file.include?("/gems/")
end

# literal_before is the literal pushed by the instruction before index, the
# last argument of the call at index, as [true, value], or [false, nil] when
# that argument is not a literal.
def literal_before(body, index)
  (index - 1).downto(0) do |at|
    instruction = body[at]
    next unless instruction.is_a?(Array)

    return instruction[0] == :putobject ? [true, instruction[1]] : [false, nil]
  end
  [false, nil]
end

# method_text is the source of a method or a block, from the line it starts on
# to the line it ends on, read back from the image.
def method_text(path, iseq)
  first, _, last, = iseq[4][:code_location]
  return nil unless path && first && last && File.readable?(path)

  File.readlines(path)[(first - 1)..(last - 1)].to_a.join
end

def line_text(path, line)
  return "" unless path && File.readable?(path)

  File.readlines(path)[line - 1].to_s
end

# balanced_from is the bracketed text that opens at open_at.
def balanced_from(text, open_at)
  depth = 0
  text[open_at..].each_char.with_index do |char, offset|
    depth += 1 if "({[".include?(char)
    depth -= 1 if ")}]".include?(char)
    return text[open_at, offset + 1] if depth.zero?
  end
  ""
end

def hash_keys(text)
  text.scan(/(?<![:\w])(\w+):(?!:)/).flatten.uniq - NON_POSITIONAL_KEYS
end

# next_link_keys names the cursor of a next link a route writes itself, from
# the hash it hands add_next_page_header: written at the call, or assigned to
# the variable the call passes earlier in the same method.
def next_link_keys(text, line)
  call_at = line.index("add_next_page_header(")
  return [] unless call_at

  argument = balanced_from(line, call_at + "add_next_page_header".length)[1..-2].to_s.strip
  return hash_keys(argument) if argument.start_with?("{") || argument.match?(/\A\w+:/)
  return [] unless text && argument.match?(/\A\w+\z/)

  assigned_at = text.index(/\b#{Regexp.escape(argument)}\s*=\s*\{/)
  return [] unless assigned_at

  hash_keys(balanced_from(text, text.index("{", assigned_at)))
end

# KeysetWalk collects the keyset sites one route's block reaches.
class KeysetWalk
  attr_reader :sites

  def initialize(helpers, paginators)
    @helpers = helpers
    @paginators = paginators
    @seen = Set.new
    @sites = []
  end

  def walk(method, via, flags)
    iseq = RubyVM::InstructionSequence.of(method)
    return unless iseq

    key = [method.source_location, method.name]
    return if @seen.include?(key)

    @seen << key
    form = iseq.to_a
    scan(form, method, via, flags, method_text(form[7], form))
  end

  private

  # helper is the method a call on the endpoint resolves to, through the
  # helpers the route is served with, or nil when they define none of that
  # name.
  def helper(name)
    return nil unless @helpers.method_defined?(name) || @helpers.private_method_defined?(name)

    @helpers.instance_method(name)
  end

  # flag_of describes a feature flag a site is behind, with whether GitLab
  # turns it on unless an administrator turns it off.
  def flag_of(name)
    entry = { "name" => name }
    definition = ::Feature::Definition.get(name.to_sym)
    entry["default_enabled"] = definition.default_enabled unless definition.nil?
    entry
  end

  def site(paginator, path, line, via, flags, cursor)
    entry = { "paginator" => paginator, "file" => repo_relative(path), "line" => line }
    # The one line the call is on rather than read_source's balanced span: a
    # call is not a lambda, and the span runs on into the line after it.
    text = squeeze(line_text(path, line))
    entry["text"] = text unless text.empty?
    entry["via"] = via unless via.empty?
    entry["cursor"] = cursor unless cursor.empty?
    entry["flags"] = flags.uniq.map { |name| flag_of(name) } unless flags.empty?
    @sites << entry
  end

  # ScanState is what scan carries from one instruction of a sequence to the
  # next: the method the sequence belongs to and the route's path to it, the
  # flags the enclosing sequences are behind, the method's text, the line the
  # instructions are on, the guards still open, and how far a flag test has
  # been read (a Feature constant, then its flag, then the enabled? or
  # disabled? call the next branch tests).
  ScanState = Struct.new(
    :owner, :via, :inherited, :text, :path, :line, :body, :guards, :feature, :flag, :flag_call,
    keyword_init: true,
  )

  # scan reads one instruction sequence, and every block nested in it, for
  # keyset sites and for the helpers it calls.
  #
  # A feature flag is recorded on the sites it guards in the one shape that
  # reads unambiguously from the instructions, `if Feature.enabled?(:flag)` or
  # `unless Feature.disabled?(:flag)`: the branch that skips the guarded code
  # jumps to a label after it, and every site met before that label is behind
  # the flag. A flag tested any other way, an early return among them, is not
  # recorded.
  def scan(form, method, via, inherited, text)
    state = ScanState.new(
      owner: method, via: via, inherited: inherited, text: text,
      path: form[7], line: form[8], body: form[13], guards: [], feature: false,
    )
    state.body.each_with_index do |instruction, index|
      case instruction
      when Integer then state.line = instruction
      when Symbol then state.guards.reject! { |guard| guard[1] == instruction }
      when Array then scan_instruction(state, instruction, index)
      end
    end
  end

  # scan_instruction reads one instruction: the blocks nested in its operands,
  # the flag test it reads a step of or closes into a guard, and the call it
  # makes, which can be a keyset site or a method to walk into.
  def scan_instruction(state, instruction, index)
    active = state.inherited + state.guards.map(&:first)
    instruction.each { |operand| scan(operand, state.owner, state.via, active, state.text) if iseq_array?(operand) }
    operation = instruction[0]
    open_guard(state, operation, instruction)
    read_operand(state, operation, instruction, active)

    calldata = instruction[1]
    return unless calldata.is_a?(Hash) && calldata[:mid]

    read_flag_call(state, calldata[:mid])
    record_site(state, calldata, index, active)
    follow_call(state, operation, calldata, active)
  end

  # open_guard opens a guard when the instruction is the branch that tests the
  # flag call the instruction before it made. A flag call is tested by the
  # next instruction or by none.
  def open_guard(state, operation, instruction)
    tested = state.flag_call
    state.flag_call = nil
    return unless (operation == :branchunless && tested&.first == :enabled?) ||
                  (operation == :branchif && tested&.first == :disabled?)

    state.guards << [tested.last, instruction[1]]
  end

  # read_operand reads a constant, which starts a flag test when it is
  # Feature and is a site when it is the Gitaly keyset pager, and the symbol a
  # flag test passes after its Feature constant.
  def read_operand(state, operation, instruction, active)
    if operation == :opt_getconstant_path
      name = instruction[1].reject { |segment| segment.to_s.empty? }.join("::")
      state.feature = name == "Feature"
      state.flag = nil
      if name.end_with?("Gitlab::Pagination::GitalyKeysetPager")
        site("gitaly_keyset_pager", state.path, state.line, state.via, active, @paginators["gitaly_keyset_pager"])
      end
    elsif operation == :putobject && state.feature && state.flag.nil? && instruction[1].is_a?(Symbol)
      state.flag = instruction[1].to_s
    end
  end

  # read_flag_call ends a flag test at its enabled? or disabled? call, for
  # the next instruction to branch on.
  def read_flag_call(state, called)
    return unless %i[enabled? disabled?].include?(called) && state.feature && state.flag

    state.flag_call = [called, state.flag]
    state.feature = false
    state.flag = nil
  end

  # follow_call walks into the method a call reaches: the parent's for a call
  # to super, and the helper's for a call on self.
  def follow_call(state, operation, calldata, active)
    if operation == :invokesuper
      parent = state.owner.super_method
      walk(parent, state.via, active) if parent && gitlab_method?(parent)
    elsif on_self?(calldata)
      called = calldata[:mid]
      callee = helper(called)
      walk(callee, state.via + [called.to_s], active) if callee && gitlab_method?(callee)
    end
  end

  def on_self?(calldata)
    (calldata[:flag].to_i & VM_CALL_FCALL) != 0
  end

  def record_site(state, calldata, index, flags)
    called = calldata[:mid]
    last_keyword = Array(calldata[:kw_arg]).last
    path = state.path
    line = state.line
    # paginate_with_strategies is given CursorPager's cursor whenever the call
    # does not pass `use_cursor: false`, which assumes a relation whose model
    # declares keyset orderings: the model is not in the instructions, and on
    # one declaring none GitLab answers 405 instead (see the section above).
    if called == :paginate_with_strategies && on_self?(calldata)
      without_cursor = last_keyword == :use_cursor && literal_before(state.body, index) == [true, false]
      site("paginate_with_strategies", path, line, state.via, flags,
        without_cursor ? @paginators["pager"] : @paginators["cursor_pager"])
    elsif called == :execute && last_keyword == :gitaly_pagination && literal_before(state.body, index) == [true, true]
      site("gitaly_pagination", path, line, state.via, flags, @paginators["gitaly_keyset_pager"])
    elsif called == :add_next_page_header
      site("keyset_header_builder", path, line, state.via, flags, next_link_keys(state.text, line_text(path, line)))
    elsif called == :add_prev_and_next_cursor_headers
      site("keyset_header_builder", path, line, state.via, flags, @paginators["cursor_headers"])
    elsif called == :keyset_paginate
      cursor = line_text(path, line)[/keyset_paginate\(\s*cursor:\s*params\[:(\w+)\]/, 1]
      site("keyset_paginate", path, line, state.via, flags, cursor ? [cursor] : [])
    end
  end
end

# keyset_of is where a route's code calls a keyset paginator, or nil when the
# code it reaches calls none.
#
# The helpers are built the way Grape builds them when it mounts the endpoint,
# because a runner never serves a request and so never mounts one: the
# endpoint's own copy is still unset here.
def keyset_of(route, paginators)
  endpoint = route.app
  source = endpoint.respond_to?(:source) ? endpoint.source : nil
  return nil unless source

  walk = KeysetWalk.new(endpoint.send(:build_helpers) || Module.new, paginators)
  walk.walk(source, [], [])
  walk.sites.empty? ? nil : walk.sites.uniq
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

# PENDING_LIST_FILE is the list GitLab keeps of the GraphQL types and
# mutations it has not declared a fine-grained permission for yet, which the
# record calls its pending list.
PENDING_LIST_FILE = "config/authz/graphql/authorization_todo.txt"

# pending_rule_type? is the type half of the rule that generates the pending
# list (lib/tasks/gitlab/permissions/graphql/schema_directives.rb).
def pending_rule_type?(name, type)
  return false if name.start_with?("__")
  return false if %w[Mutation Query Subscription].include?(name)

  type.kind.object? && !name.end_with?("Payload", "Connection", "Edge")
end

# pending_list_document reads the pending list, when the image ships it, so
# the set computed below can be held to it.
def pending_list_document
  path = Rails.root.join(PENDING_LIST_FILE)
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

# abstract_of records a union or an interface: which of the two it is, and the
# object types GitLab may resolve it to.
def abstract_of(type)
  {
    "kind" => type.kind.union? ? "union" : "interface",
    "possible_types" => GitlabSchema.possible_types(type).map(&:graphql_name).sort,
  }
end

# graphql_named_types reads every named type but the introspection ones: an
# object into what it declares, a union or an interface into the types it
# resolves to, and the names of the objects the pending-list rule counts as
# declaring nothing.
def graphql_named_types
  types = {}
  abstract = {}
  rule_types = []
  GitlabSchema.types.sort.each do |name, type|
    next if name.start_with?("__")

    kind = type.kind
    if kind.object?
      types[name] = type_of(type)
      rule_types << name if pending_rule_type?(name, type) && type.directives.none? { |d| d.is_a?(GRANULAR_SCOPE) }
    elsif kind.union? || kind.interface?
      abstract[name] = abstract_of(type)
    end
  end
  [types, abstract, rule_types]
end

# graphql_field_declarations reads the directives declared on a field, of
# every type but Mutation, whose fields are read as mutations.
def graphql_field_declarations
  fields = {}
  GitlabSchema.types.sort.each do |name, type|
    next if name.start_with?("__") || name == "Mutation" || !type.respond_to?(:fields)

    type.fields.sort.each do |field_name, field|
      granular = granular_of(field)
      fields["#{name}.#{field_name}"] = granular unless granular.empty?
    end
  end
  fields
end

# mutation_entry_of records one mutation: its names, its payload, what
# GitLab's runtime check reads and, where the permission task would read
# something else, what that is.
def mutation_entry_of(resolver, granular, task_granular)
  entry = {
    "name" => resolver.graphql_name.to_s,
    "class" => resolver.name.to_s,
    "payload" => resolver.payload_type.graphql_name.to_s,
  }
  entry["granular"] = granular unless granular.empty?
  entry["field_granular"] = task_granular unless task_granular.empty? || task_granular == granular
  entry
end

# graphql_mutations reads every field of Mutation a Mutations::BaseMutation
# backs, and the names of the mutations the pending-list rule counts as
# declaring nothing.
def graphql_mutations
  mutations = {}
  rule_mutations = []
  GitlabSchema.types["Mutation"].fields.sort.each do |field_name, field|
    resolver = mutation_class_of(field)
    next unless resolver

    # granular is what GitLab's runtime check reads, the class's own
    # directives and nothing else (Mutations::BaseMutation#granular_scope_authorization
    # hands GranularScopeAuthorization self.class.directives).
    #
    # GitLab's permission task reads the field's directives first and the
    # class's only when the field has none (find_mutation_directives in
    # lib/tasks/gitlab/permissions/graphql/schema_directives.rb), and the
    # pending list is generated that way, so the rule below keeps that
    # reading. graphql-ruby's Schema::Field#directives (2.6.10 at 19.4.1)
    # answers for a mutation field with any directives the field declares
    # itself followed by the class's, and mount_mutation declares none on the
    # field, so the two readings agree at every mutation today.
    # field_granular is recorded only where they do not, and the gate refuses
    # it, because the pending list and what a fine-grained token is refused
    # would then describe two requirements.
    granular = granular_of(resolver)
    task_granular = granular_of(field)
    mutations[field_name] = mutation_entry_of(resolver, granular, task_granular)
    rule_mutations << resolver.graphql_name.to_s if task_granular.empty? && granular.empty?
  end
  [mutations, rule_mutations]
end

def graphql_authz_document
  types, abstract, rule_types = graphql_named_types
  mutations, rule_mutations = graphql_mutations
  document = {
    "types" => types,
    "abstract" => abstract,
    "mutations" => mutations,
    "fields" => graphql_field_declarations,
    "undeclared_by_todo_rule" => { "types" => rule_types.sort, "mutations" => rule_mutations.sort },
  }
  pending = pending_list_document
  document["todo"] = pending if pending
  document
end

# ---------------------------------------------------------------------------

entities = entities_document
keyset_paginators = keyset_paginators_document
routes = routes_document(keyset_paginators)
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
  "keyset_paginators" => keyset_paginators,
)
