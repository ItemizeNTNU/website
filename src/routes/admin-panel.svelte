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
		const dbApplications = await api.getAllApplications({ fetch: this.fetch });
		const dbGroups = await api.getAllGroups({ fetch: this.fetch });
		return { users: dbUsers.json.users, applications: dbApplications.json.applications, groups: dbGroups.json.groups };
	}
</script>

<script>
	import AdminPanelTable from '../components/AdminPanelTable.svelte';
	import date from '../utils/date';
	export let users;
	export let applications;
	export let groups;
	console.log(users);
	console.log(applications);
	console.log(groups);
	let selectedCols = ['fullName', 'discordName', 'email', 'type', 'roles'];
	let selection = { fullName: '', discordName: '', email: '' };
	const COLUMNS = {
		fullName: {
			key: 'fullName',
			title: 'Navn',
			value: (v) => v.fullName,
			sortable: true,
			searchValue: (v) => v.fullName
		},
		discordName: {
			key: 'discordName',
			title: 'Discord-navn',
			value: (v) => v.discordUsername || '',
			sortable: true,
			searchValue: (v) => v.discordUsername
		},
		email: {
			key: 'email',
			title: 'E-post',
			value: (v) => v.email,
			sortable: true,
			searchValue: (v) => v.email
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
	const filterOptions = {
		type: ['alumni', 'ansatt', 'student']
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
			<h2>Admin panel</h2>

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
				{filterOptions}
				on:clickExpand={handleExpand}
				bind:filterSelections={selection}
			>
				<div slot="expanded" let:row class="user-info">
					<p><b>Fullt Navn: </b>{row.fullName}</p>
					<p><b>Visningsnavn: </b>{row.name}</p>
					<p><b>E-post: </b>{row.email}</p>
					<p><b>Medlemstype: </b>{row.type}</p>
					{#if row.type == 'student' || row.type == 'alumni'}
						<p><b>Studieretning: </b>{row.study.program}</p>
						{#if row.type == 'student'}
							<p><b>Årstrinn: </b>{row.study.year}</p>
						{:else}
							<p><b>Medlemsår: </b>{row.alumni.joinYear}</p>
						{/if}
					{:else if row.type == 'employee'}
						<p><b>Title: </b>{row.employee.title}</p>
						<p />
					{/if}
					<p><b>Bruker opprettet: </b>{date.nicePrintDate(new Date(row.insertInstant))}</p>
					<p><b>Sist logget in: </b>{date.nicePrintDate(new Date(row.lastLoginInstant))}</p>
				</div>
			</AdminPanelTable>
		</div>
	</div>
</main>

<style>
	.user-info {
		display: grid;
		grid-template-columns: 50% 50%;
		align-items: center;
		padding: 0.6em;
	}
	.user-info * {
		margin: 0.1em 0;
	}
	.user-info p {
		width: 100%;
	}
</style>
