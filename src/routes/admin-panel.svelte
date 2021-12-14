<script context="module">
	import api from '../utils/api';

	export async function preload(page, session) {
		if (!session.user) {
			this.redirect(302, '/login');
		} else if (!session.user?.roles.includes('Styret')) {
			this.redirect(401, '/');
		}
		const query = 'queryString=*';
		const dbUsers = await api.searchUsers(query, { fetch: this.fetch });
		console.log(dbUsers.json.users);
		return { users: dbUsers.json.users };
	}
</script>

<script>
	import AdminPanelTable from '../components/AdminPanelTable.svelte';
	export let users;
	console.log(users);
	console.log(Object.values(users));
	let selectedCols = ['fullName', 'discordName', 'email', 'type', 'roles'];

	const COLUMNS = {
		fullName: {
			key: 'fullName',
			title: 'Navn',
			value: (v) => v.fullName,
			sortable: true
		},
		discordName: {
			key: 'discordName',
			title: 'Discord-navn',
			value: (v) => v.discordUsername || '',
			sortable: true
		},
		email: {
			key: 'email',
			title: 'E-Post',
			value: (v) => v.email,
			sortable: true
		},
		type: {
			key: 'type',
			title: 'Medlemstype',
			value: (v) => v.type || '',
			sortable: true
		},
		roles: {
			key: 'roles',
			title: 'Roller',
			value: (v) => v.roles,
			sortable: true
		}
	};

	$: cols = selectedCols.map((key) => COLUMNS[key]);

	function handleExpand(event) {
		const row = event.detail.row;
		console.log(row);
		//const operation = row.$expanded ? "open" : "close";
	}
</script>

<svelte:head>
	<title>Admin panel</title>
</svelte:head>

<main>
	<div class="container">
		<div class="row">
			<h2>Expand row example 2</h2>

			<p>
				Only 1 row can be expanded at a time<br />
				Console logs selection change
			</p>

			<AdminPanelTable
				columns={cols}
				rows={users}
				classNameTable="table"
				classNameThead="table-info"
				showExpandIcon={true}
				expandSingle={true}
				expandRowKey="fullName"
				iconExpand="⌄"
				iconExpanded="⌃"
				on:clickExpand={handleExpand}
			>
				<div slot="expanded" let:row class="text-center">
					{row.county}, {row.state}<br />
					{row.country}
				</div>
			</AdminPanelTable>
		</div>
	</div>
</main>
