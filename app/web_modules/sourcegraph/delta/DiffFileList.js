import React from "react";

import classNames from "classnames";
import DiffStatScale from "sourcegraph/delta/DiffStatScale";
import {isDevNull} from "sourcegraph/delta/util";

class DiffFileList extends React.Component {
	constructor(props) {
		super(props);
		this.state = {closed: false};
	}

	render() {
		return (
			<div className={classNames({
				"file-list": true,
				"closed": Boolean(this.state.closed),
			})}>
				<a className="file-list-header" onClick={() => this.setState({closed: !this.state.closed})}>
					<i className={this.state.closed ? "fa fa-icon fa-plus-square-o" : "fa fa-icon fa-minus-square-o"} />
					<b>Files</b> <span className="count">( {this.props.files.length} )</span>
					<span className="pull-right stats">
						<span className="additions-color">+{this.props.stats.Added}</span>
						<span className="deletions-color">-{this.props.stats.Deleted}</span>
					</span>
				</a>

				<ul className="file-list-items">
					{this.props.files.map((fd, i) => (
						<li key={fd.OrigName + fd.NewName} className="file-list-item">
							<a href={`#F${i}`}>
								{isDevNull(fd.OrigName) ? <code className="change-type additions-color">+</code> : null}
								{isDevNull(fd.NewName) ? <code className="change-type deletions-color">&minus;</code> : null}
								{!isDevNull(fd.OrigName) && !isDevNull(fd.NewName) ? <code className="change-type changes-color">&bull;</code> : null}

								{!isDevNull(fd.OrigName) && !isDevNull(fd.NewName) && fd.OrigName !== fd.NewName ? (
									<span>{fd.OrigName} <i className="fa fa-icon fa-long-arrow-right" />&nbsp;</span>
								) : null}

								{isDevNull(fd.NewName) ? fd.OrigName : fd.NewName}

								<div className="pull-right stats">
									<span className="additions-color">+{this.props.stats.Added}</span>
									<span className="deletions-color">-{this.props.stats.Deleted}</span>
									<DiffStatScale Stat={this.props.stats} />
								</div>
							</a>
						</li>
					))}
				</ul>
			</div>
		);
	}
}
DiffFileList.propTypes = {
	files: React.PropTypes.arrayOf(React.PropTypes.object),
	stats: React.PropTypes.object.isRequired,
};
export default DiffFileList;
