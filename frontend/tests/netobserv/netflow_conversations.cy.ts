import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, colSelectors, querySumSelectors } from "../../views/netflow-page"

describe('(OCP-71787 Network_Observability) Conversation tracking test', { tags: ['Network_Observability'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
        cy.switchPerspective('Administrator');

        // specify --env noo_release=upstream to run tests 
        // from most recent "main" image
        let catalogImg
        let catalogDisplayName = "Production Operators"
        const catSrc = Cypress.env('noo_catalog_src')
        if (catSrc == "upstream") {
            catalogImg = 'quay.io/netobserv/network-observability-operator-catalog:v0.0.0-main'
            this.catalogSource = "netobserv-test"
            catalogDisplayName = "NetObserv QE"
            catalogSources.createCustomCatalog(catalogImg, this.catalogSource, catalogDisplayName)
        }
        else {
            catalogSources.enableQECatalogSource(this.catalogSource, catalogDisplayName)
        }

        Operator.install(catalogDisplayName)
        Operator.createFlowcollector(project, "Conversations")
    })

    beforeEach('any conversation tracking test', function () {
        netflowPage.visit()
        cy.get('#tabs-container li:nth-child(2)').click()
        cy.byTestID("table-composable").should('exist')
    })

    it("(OCP-71787, aramesha, Network_Observability) should validate default conversation tracking columns", function () {
        cy.byTestID('table-composable').should('exist').within(() => {
            cy.get(colSelectors.RecordType).should('exist')
            cy.get(colSelectors.conversationID).should('exist')
        })
    })

    it("(OCP-71787, aramesha, Network_Observability) should verify Query Summary panel", function () {
        cy.get('#filter-toolbar-search-filters').contains('Query options').click();
        cy.get('#query-options-dropdown').click();
        cy.get('#recordType-allConnections').click()
        cy.get('#filter-toolbar-search-filters').contains('Query options').click();

        // validate Query Summary panel
        let warningExists = false
        cy.get(querySumSelectors.queryStatsPanel).should('exist').then(qrySum => {
            if (Cypress.$(querySumSelectors.queryStatsPanel + ' svg.query-summary-warning').length > 0) {
                warningExists = true
            }
        })

        cy.get(querySumSelectors.flowsCount).should('exist').then(ConversationsCnt => {
            let nflows = 0
            if (warningExists) {
                nflows = Number(ConversationsCnt.text().split('+ Ended conversations')[0])
            }
            else {
                nflows = Number(ConversationsCnt.text().split(' ')[0])
            }
            cy.wait(10)
            expect(nflows).to.be.gte(0)
        })
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("delete flowcollector and NetObs Operator", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
