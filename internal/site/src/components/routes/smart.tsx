import { useEffect } from "react"
import SmartTable from "@/components/routes/system/smart-table"
import { FooterRepoLink } from "@/components/footer-repo-link"

export default function Smart() {
	useEffect(() => {
		document.title = `S.M.A.R.T. / Beszel`
	}, [])

	return (
		<>
			<SmartTable />
			<FooterRepoLink />
		</>
	)
}
