package deal

import (
	"github.com/action-state-group/evidencebook/dealprofile/xdealv1"
)

// verifyV1Report is the x-deal-v1 report rule the core's `verify --bundle`
// runs through cli.SetDealV1ReportVerifier (evidencebook-deal#233): the
// core checks a v1 report's neutral part and cannot import EvidenceBook, so
// without this a passing report reads report_rule_not_checked. report is the
// report pair and first the deal book's record at seq 1, both as
// xdealv1.VerifyReport takes them; the returned report is not needed here,
// the core shows the report it already read.
func verifyV1Report(report, first []byte) error {
	_, err := xdealv1.VerifyReport(report, first)
	return err
}
