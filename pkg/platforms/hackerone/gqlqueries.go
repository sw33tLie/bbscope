package hackerone

var completeHacktivitySearchQuery = `
query CompleteHacktivitySearchQuery($queryString: String!, $from: Int, $size: Int, $sort: SortInput!) {
  me {
    id
    __typename
  }
  search(
    index: CompleteHacktivityReportIndexService
    query_string: $queryString
    from: $from
    size: $size
    sort: $sort
  ) {
    __typename
    total_count
    nodes {
      __typename
      ... on CompleteHacktivityReportDocument {
        id
        _id
        reporter {
          id
          name
          username
          ...UserLinkWithMiniProfile
          __typename
        }
        cve_ids
        cwe
        severity_rating
        upvoted: upvoted_by_current_user
        public
        report {
          id
          databaseId: _id
          title
          substate
          url
          disclosed_at
          report_generated_content {
            id
            hacktivity_summary
            __typename
          }
          __typename
        }
        votes
        team {
          handle
          name
          medium_profile_picture: profile_picture(size: medium)
          url
          id
          currency
          ...TeamLinkWithMiniProfile
          __typename
        }
        total_awarded_amount
        latest_disclosable_action
        latest_disclosable_activity_at
        submitted_at
        disclosed
        has_collaboration
        __typename
      }
    }
  }
}

fragment UserLinkWithMiniProfile on User {
  id
  username
  __typename
}

fragment TeamLinkWithMiniProfile on Team {
  id
  handle
  name
  __typename
}
`
