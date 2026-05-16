package webapi

import (
	"fmt"
	"net/http"

	"github.com/go-pkgz/rest"
	"github.com/go-pkgz/routegroup"
)

func (s *Server) routes(router *routegroup.Bundle) *routegroup.Bundle {
	s.setupAPIRoutes(router)
	s.setupWebUIRoutes(router)
	return router
}

func (s *Server) setupAPIRoutes(router *routegroup.Bundle) {
	router.Group().Route(func(authApi *routegroup.Bundle) {
		authApi.Use(s.authMiddleware(rest.BasicAuthWithUserPasswd("tg-spam", s.AuthPasswd)))
		authApi.HandleFunc("POST /check", s.checkMsgHandler)
		authApi.HandleFunc("GET /check/{user_id}", s.checkIDHandler)

		authApi.Mount("/update").Route(func(r *routegroup.Bundle) {
			r.HandleFunc("POST /spam", s.updateSampleHandler(s.SpamFilter.UpdateSpam))
			r.HandleFunc("POST /ham", s.updateSampleHandler(s.SpamFilter.UpdateHam))
		})

		authApi.Mount("/delete").Route(func(r *routegroup.Bundle) {
			r.HandleFunc("POST /spam", s.deleteSampleHandler(s.SpamFilter.RemoveDynamicSpamSample))
			r.HandleFunc("POST /ham", s.deleteSampleHandler(s.SpamFilter.RemoveDynamicHamSample))
		})

		authApi.Mount("/download").Route(func(r *routegroup.Bundle) {
			r.HandleFunc("GET /spam", s.downloadSampleHandler(func(spam, _ []string) ([]string, string) {
				return spam, "spam.txt"
			}))
			r.HandleFunc("GET /ham", s.downloadSampleHandler(func(_, ham []string) ([]string, string) {
				return ham, "ham.txt"
			}))
			r.HandleFunc("GET /detected_spam", s.downloadDetectedSpamHandler)
			r.HandleFunc("GET /backup", s.downloadBackupHandler)
			r.HandleFunc("GET /export-to-postgres", s.downloadExportToPostgresHandler)
		})

		authApi.HandleFunc("GET /samples", s.getDynamicSamplesHandler)
		authApi.HandleFunc("PUT /samples", s.reloadDynamicSamplesHandler)

		authApi.Mount("/users").Route(func(r *routegroup.Bundle) {
			au := s.approvedUsers()
			r.HandleFunc("POST /add", s.updateApprovedUsersHandler(au.Add))
			r.HandleFunc("POST /delete", s.updateApprovedUsersHandler(s.removeApprovedUserAdapter))
			r.HandleFunc("GET /", s.getApprovedUsersHandler)
		})

		authApi.HandleFunc("GET /settings", s.getSettingsHandler)

		authApi.Mount("/rules").Route(func(r *routegroup.Bundle) {
			r.Use(s.controlPlaneAuthMiddleware)
			r.HandleFunc("GET /", s.getRuleSetHandler)
			r.HandleFunc("PUT /", s.updateRuleSetHandler)
		})

		authApi.Mount("/dictionary").Route(func(r *routegroup.Bundle) {
			r.HandleFunc("POST /add", s.addDictionaryEntryHandler)
			r.HandleFunc("POST /delete", s.deleteDictionaryEntryHandler)
			r.HandleFunc("GET /", s.getDictionaryEntriesHandler)
		})

		if s.AuditService != nil {
			authApi.Mount("/api/incidents").Route(func(r *routegroup.Bundle) {
				r.HandleFunc("GET /", s.listIncidentsHandler)
				r.HandleFunc("GET /{id}", s.getIncidentHandler)
				r.HandleFunc("POST /{id}/replay", s.replayIncidentHandler)
				r.HandleFunc("POST /{id}/comment", s.addIncidentCommentHandler)
				r.HandleFunc("POST /{id}/status", s.updateIncidentStatusHandler)
			})
		}
		if s.AppealService != nil {
			authApi.Mount("/api/appeals").Route(func(r *routegroup.Bundle) {
				r.HandleFunc("GET /", s.listAppealsHandler)
				r.HandleFunc("POST /{id}/resolve", s.resolveAppealHandler)
			})
		}
		if s.FeedbackService != nil {
			authApi.Mount("/api/feedback").Route(func(r *routegroup.Bundle) {
				r.HandleFunc("POST /labels", s.createLabelHandler)
				r.HandleFunc("GET /labels", s.listLabelsHandler)
				r.HandleFunc("GET /labels/stats", s.labelStatsHandler)

				if s.ReviewService != nil {
					r.HandleFunc("GET /candidates", s.listCandidatesHandler)
					r.HandleFunc("POST /candidates/{id}/approve", s.approveCandidateHandler)
					r.HandleFunc("POST /candidates/{id}/reject", s.rejectCandidateHandler)
					r.HandleFunc("POST /candidates/generate", s.generateCandidatesHandler)
				}

				if s.KnowledgeService != nil {
					r.HandleFunc("POST /knowledge/snapshots", s.createKnowledgeSnapshotHandler)
					r.HandleFunc("GET /knowledge/snapshots", s.listKnowledgeSnapshotsHandler)
					r.HandleFunc("GET /knowledge/snapshots/{id}", s.getKnowledgeSnapshotHandler)
					r.HandleFunc("POST /knowledge/snapshots/{id}/rollback", s.rollbackKnowledgeHandler)
				}
			})
		}
		if s.MetricsCollector != nil {
			authApi.HandleFunc("GET /api/metrics", s.metricsHandler)
		}
		if s.OnboardingProvider != nil || s.RestoreProvider != nil {
			authApi.Mount("/api/tenants").Route(func(r *routegroup.Bundle) {
				if s.OnboardingProvider != nil {
					r.HandleFunc("POST /onboard", s.onboardTenantHandler)
					r.HandleFunc("POST /{id}/offboard", s.offboardTenantHandler)
				}
				if s.RestoreProvider != nil {
					r.HandleFunc("POST /{id}/restore", s.restoreTenantHandler)
				}
			})
		}
	})
}

func (s *Server) setupWebUIRoutes(router *routegroup.Bundle) {
	router.Group().Route(func(webUI *routegroup.Bundle) {
		webUI.Use(s.authMiddleware(rest.BasicAuthWithPrompt("tg-spam", s.AuthPasswd)))
		webUI.HandleFunc("GET /", s.htmlSpamCheckHandler)
		webUI.HandleFunc("GET /manage_samples", s.htmlManageSamplesHandler)
		webUI.HandleFunc("GET /manage_users", s.htmlManageUsersHandler)
		webUI.HandleFunc("GET /manage_dictionary", s.htmlManageDictionaryHandler)
		webUI.HandleFunc("GET /detected_spam", s.htmlDetectedSpamHandler)
		webUI.HandleFunc("GET /list_settings", s.htmlSettingsHandler)
		webUI.HandleFunc("GET /settings/edit", s.htmlSettingsEditHandler)
		webUI.HandleFunc("POST /settings/save", s.saveSettingsHandler)
		webUI.HandleFunc("POST /detected_spam/add", s.htmlAddDetectedSpamHandler)
		webUI.HandleFunc("GET /dm-users", s.getDMUsersHandler)

		if s.AuditService != nil {
			webUI.HandleFunc("GET /incidents", s.htmlIncidentsHandler)
			webUI.HandleFunc("GET /incidents/{id}", s.htmlIncidentDetailHandler)
			webUI.HandleFunc("GET /appeals", s.htmlAppealsHandler)
			webUI.HandleFunc("GET /feedback", s.htmlFeedbackHandler)
		}

		webUI.HandleFunc("GET /logout", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("WWW-Authenticate", `Basic realm="tg-spam"`)
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, "Logged out successfully")
		})

		staticFiles := newStaticFS(templateFS,
			staticFileMapping{urlPath: "styles.css", filesysPath: "assets/styles.css"},
			staticFileMapping{urlPath: "logo.png", filesysPath: "assets/logo.png"},
			staticFileMapping{urlPath: "spinner.svg", filesysPath: "assets/spinner.svg"},
		)
		webUI.HandleFiles("/", http.FS(staticFiles))
	})
}
