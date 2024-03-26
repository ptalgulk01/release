import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"
import { netflowPage, genSelectors, querySumSelectors, overviewSelectors } from "../../views/netflow-page"

describe('(OCP-54839 Network_Observability) Netflow Overview page tests', { tags: ['Network_Observability'] }, function () {

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
        Operator.createFlowcollector(project)
    })

    describe("overview page features", function () {
        beforeEach('overview page test', function () {
            netflowPage.visit()
            netflowPage.waitForLokiQuery()
            cy.get('.overviewTabButton').should('exist')

            cy.checkPanel(overviewSelectors.defaultPanels)
            cy.checkPanelsNum();
        })

        it("(OCP-54839, aramesha, Network_Observability) should validate overview page features", { tags: ['e2e', 'admin', '@smoke'] }, function () {
            cy.byTestID(genSelectors.timeDrop).then(btn => {
                expect(btn).to.exist
                cy.wrap(btn).click().then(drop => {
                    cy.byTestID('1h').should('exist').click()
                })
            })

            cy.byTestID(genSelectors.refreshDrop).then(btn => {
                expect(btn).to.exist
                cy.wrap(btn).click().then(drop => {
                    cy.byTestID('15s').should('exist').click()
                })
            })

            cy.byTestID(genSelectors.refreshBtn).should('exist').click()

            cy.showAdvancedOptions().then(views => {
                cy.contains('Display options').should('exist').click()

                // validate scope dropdown
                cy.byTestID(overviewSelectors.scopeDrop).then(btn => {
                    expect(btn).to.exist
                    cy.wrap(btn).click().then(drop => {
                        cy.byTestID('host').should('exist')
                        cy.byTestID('namespace').should('exist')
                        cy.byTestID('owner').should('exist')
                        cy.byTestID('resource').should('exist').click()
                    })
                })

                // verify truncate labels dropdown
                cy.byTestID(overviewSelectors.truncateDrop).then(btn => {
                    expect(btn).to.exist
                    cy.wrap(btn).click().then(drop => {
                        cy.byTestID('0').should('exist')
                        cy.byTestID('10').should('exist')
                        cy.byTestID('20').should('exist')
                        cy.byTestID('30').should('exist')
                        cy.byTestID('40').should('exist')
                        cy.byTestID('25').should('exist').click()
                    })
                })

                // verify single focus graph toggle
                cy.get('#focus-switch').check()
                // verify 'Top 5 average bytes rates' panel is in focus
                cy.get('#overview-absolute-graph').contains('Top 5 average bytes rates');
                cy.get('[data-test-metrics]').its('length').should('gt', 0);

                // verify other panels are scrollable on the right
                for (let i = 0; i < overviewSelectors.defaultPanels.length; i++) {
                    cy.get('#overview-graph-list').contains(overviewSelectors.defaultPanels[i])
                }

                // uncheck single focus toggle and verify panels
                cy.get('#focus-switch').check()
                cy.checkPanel(overviewSelectors.defaultPanels)
                cy.checkPanelsNum();
            })
        })

        it("(OCP-54839, aramesha, Network_Observability) should validate query summary panel", function () {
            cy.get(querySumSelectors.bytesCount).should('exist').then(bytesCnt => {
                cy.checkQuerySummary(bytesCnt)
            })

            cy.get(querySumSelectors.bpsCount).should('exist').then(bpsCnt => {
                cy.checkQuerySummary(bpsCnt)
            })
            cy.get('#query-summary-toggle').should('exist').click()
            cy.get('#summaryPanel').should('be.visible')

            cy.contains('Results').should('exist')
            cy.contains('Cardinality').should('exist')
            cy.contains('Configuration').should('exist')
            cy.contains('Sampling').should('exist')
        })

        it("(OCP-54839, aramesha, Network_Observability) should validate panels", { tags: ['e2e', 'admin', '@smoke'] }, function () {
            // open panels modal
            cy.openPanelsModal();

            // verify all panels are listed  
            cy.checkPopupItems(overviewSelectors.panelsModal, overviewSelectors.managePanelsList);

            // select all panels
            cy.get(overviewSelectors.panelsModal).contains('Select all').click();
            cy.get(overviewSelectors.panelsModal).contains('Save').click();
            netflowPage.waitForLokiQuery()
            cy.checkPanelsNum(4);

            // verify all panels are rendered
            netflowPage.waitForLokiQuery()
            cy.checkPanel(overviewSelectors.allPanels)

            // unselect all panels and verify save is disabled
            cy.openPanelsModal();
            cy.get(overviewSelectors.panelsModal).contains('Unselect all').click();
            cy.get(overviewSelectors.panelsModal).contains('Save').should('be.disabled');

            // select 1 panel and verify its visible on console
            cy.selectPopupItems(overviewSelectors.panelsModal, ['Top X average packets rates (donut)']);
            cy.get(overviewSelectors.panelsModal).contains('Save').click();
            netflowPage.waitForLokiQuery()
            cy.checkPanel([overviewSelectors.allPanels[2]])
            cy.checkPanelsNum(1);

            // restore default panels and verify visible on console
            cy.openPanelsModal();
            cy.get(overviewSelectors.panelsModal).contains('Restore default panels').click();
            cy.get(overviewSelectors.panelsModal).contains('Save').click();
            netflowPage.waitForLokiQuery()
            cy.checkPanel(overviewSelectors.defaultPanels)
            cy.checkPanelsNum();
        })
    })

    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("after all tests are done", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
