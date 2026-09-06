// Package achievements implements MCP tools for GitLab achievements: the
// badges a namespace defines and the awards handed out from them.
//
// The endpoints exist only in GraphQL, so every action here goes through the
// client-go AchievementsService rather than a REST wrapper, and the list
// actions paginate by cursor instead of by page number.
//
// The package wraps the GitLab Achievements GraphQL API:
//
//   - https://docs.gitlab.com/api/graphql/reference/#achievement
//   - https://docs.gitlab.com/api/graphql/reference/#userachievement
//   - https://docs.gitlab.com/api/graphql/reference/#namespaceachievements
//   - https://docs.gitlab.com/api/graphql/reference/#useruserachievements
//   - https://docs.gitlab.com/user/profile/achievements/
package achievements
