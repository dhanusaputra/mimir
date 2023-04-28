// SPDX-License-Identifier: AGPL-3.0-only

package querymiddleware

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/promql/parser"

	apierror "github.com/grafana/mimir/pkg/api/error"
)

type queryStatsMiddleware struct {
	nonAlignedQueries           prometheus.Counter
	regexpMatcherCount          prometheus.Counter
	regexpMatcherOptimizedCount prometheus.Counter
	next                        Handler
}

func newQueryStatsMiddleware(reg prometheus.Registerer) Middleware {
	nonAlignedQueries := promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Name: "cortex_query_frontend_non_step_aligned_queries_total",
		Help: "Total queries sent that are not step aligned.",
	})
	regexpMatcherCount := promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Name: "cortex_query_frontend_regexp_matcher_count",
		Help: "Total number of regexp matchers",
	})
	regexpMatcherOptimizedCount := promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Name: "cortex_query_frontend_regexp_matcher_optimized_count",
		Help: "Total number of optimized regexp matchers",
	})

	return MiddlewareFunc(func(next Handler) Handler {
		return &queryStatsMiddleware{
			nonAlignedQueries:           nonAlignedQueries,
			regexpMatcherCount:          regexpMatcherCount,
			regexpMatcherOptimizedCount: regexpMatcherOptimizedCount,
			next:                        next,
		}
	})
}

func (s queryStatsMiddleware) Do(ctx context.Context, req Request) (Response, error) {
	if !isRequestStepAligned(req) {
		s.nonAlignedQueries.Inc()
	}

	expr, err := parser.ParseExpr(req.GetQuery())
	if err != nil {
		return nil, apierror.New(apierror.TypeBadData, err.Error())
	}

	for _, selectors := range parser.ExtractSelectors(expr) {
		for _, matcher := range selectors {
			s.regexpMatcherCount.Inc()
			if matcher.IsRegexOptimized() {
				s.regexpMatcherOptimizedCount.Inc()
			}
		}
	}

	return s.next.Do(ctx, req)
}
